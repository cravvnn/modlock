// Drawing turns each mod's tree into stock panels. It runs no mod code: every
// property it writes comes from a typed field of the tree, and a node becomes
// one of four stock panel types. A layout node is the one exception: a host
// that has reviewed a mod's content addon passes Layouts, and the drawing
// loads the mod's layout into the node's panel; without Layouts the panel
// stays empty. It reaches the game only through the Panorama it is given, so a
// host's own interface script may draw the screens it receives another way.

import {
  Align,
  type Edges,
  Face,
  Flow,
  Kind,
  type Length,
  type Node,
  type Screen,
  type Style,
} from '../../src/wasm/ui/ui.pb.js'

/** Panel is the part of a game panel the drawing uses. */
export interface Panel {
  readonly paneltype: string
  hittest: boolean
  text: string
  readonly style: Record<string, string>
  IsValid(): boolean
  GetChild(index: number): Panel | null
  MoveChildBefore(child: Panel, before: Panel): void
  MoveChildAfter(child: Panel, after: Panel): void
  SetImage(path: string): void
  SetPanelEvent(event: string, handler: () => void): void
  SetAttributeString(name: string, value: string): void
  BLoadLayout(
    path: string,
    overrideExisting: boolean,
    partial: boolean,
  ): boolean
  DeleteAsync(seconds: number): void
}

/** Panorama is the part of the game's script API the drawing uses. */
export interface Panorama {
  CreatePanel(type: string, parent: Panel, id: string): Panel
  DispatchEvent(event: string, value: string): void
  Msg(...values: unknown[]): void
}

/** Press reports a press on a mod's button. */
export type Press = (mod: string, node: string) => void

/**
 * Layouts lets a host load the layouts of mods whose content it has reviewed.
 * A layout's script runs in the player's game, so only a host that checked
 * the mod's addon passes one.
 */
export interface Layouts {
  /**
   * url returns the URL of mod's layout at path, a path isLayoutPath accepts,
   * or null when the host does not load that mod's layouts.
   */
  url(mod: string, path: string): string | null
  /**
   * unloaded runs after the drawing deletes one or more layouts' panels. A
   * layout's key binds replaced the host's for the whole game process, so the
   * host binds its keys again.
   */
  unloaded(): void
}

/** layoutDataAttribute names the attribute holding a layout node's data. */
export const layoutDataAttribute = 'modlock_data'

/**
 * layoutDataChanged is the sound event the drawing plays after it changes a
 * layout's data. The game refuses custom events and delivers the stock
 * PlaySoundEffect to every script; no sound has this name, so none plays.
 */
export const layoutDataChanged = 'modlock_data_changed'

/**
 * isLayoutPath reports whether path names a layout inside a mod's addon: one
 * or more segments of letters, digits, '_', '-' and '.', none of them "..",
 * ending in ".xml". The screen comes from the game server, so the drawing
 * checks the path before a host joins it to the addon's layout directory.
 */
export function isLayoutPath(path: string): boolean {
  return (
    path.endsWith('.xml') &&
    path.split('/').every((part) => /^[\w.-]+$/.test(part) && part !== '..')
  )
}

/**
 * pressCommand is the console command that presses node in mod's interface.
 * The game server hears it with the sending player, so any player's game can
 * press by running it.
 */
export function pressCommand(mod: string, node: string): string {
  return `modlockpress ${mod} ${node}`
}

/** panelTypes maps each kind of node to the stock panel that draws it. */
const panelTypes: Record<Kind, string> = {
  [Kind.PANEL]: 'Panel',
  [Kind.LABEL]: 'Label',
  [Kind.IMAGE]: 'Image',
  [Kind.BUTTON]: 'Button',
  [Kind.LAYOUT]: 'Panel',
}

/**
 * Drawn is one node's panel, its parent and the style written to it. A layout
 * node's panel also records the layout's URL, whether it loaded, and the data
 * set on it.
 */
interface Drawn {
  panel: Panel
  parent: Panel
  style: Record<string, string>
  layout: string | null
  loaded: boolean
  data: string
}

/** Drawing keeps the panels of every mod's tree under one parent. */
export class Drawing {
  /** mods holds each mod's drawn nodes by id; the root's id is empty. */
  private readonly mods = new Map<string, Map<string, Drawn>>()

  constructor(
    private readonly panorama: Panorama,
    private readonly parent: Panel,
    private readonly press: Press,
    private readonly layouts?: Layouts,
  ) {}

  /** unloaded records that this draw deleted a layout's panel. */
  private unloaded = false

  /** draw makes the panels match screen, keeping each panel whose node stays. */
  draw(screen: Screen | null): void {
    // Delete the trees of mods the screen no longer holds.
    const trees = new Map(
      (screen?.trees ?? []).map((tree) => [tree.mod ?? '', tree.nodes ?? []]),
    )
    for (const [mod, drawn] of this.mods) {
      if (!trees.has(mod)) {
        drawn.get('')?.panel.DeleteAsync(0)
        this.unloaded ||= [...drawn.values()].some(
          (each) => each.layout !== null,
        )
        this.mods.delete(mod)
      }
    }

    // Draw each mod's tree.
    for (const [mod, nodes] of trees) {
      this.drawTree(mod, new Map(nodes.map((node) => [node.id ?? '', node])))
    }

    // Let the host bind its keys again once layouts are gone.
    if (this.unloaded) {
      this.unloaded = false
      this.layouts?.unloaded()
    }
  }

  /** drawTree draws one mod's nodes from its root down. */
  private drawTree(mod: string, nodes: Map<string, Node>): void {
    // Draw the mod's tree from its root, reusing the panels drawn last time.
    const previous = this.mods.get(mod) ?? new Map<string, Drawn>()
    const drawn = new Map<string, Drawn>()
    const root = nodes.get('') ?? { id: '' }
    this.drawNode(mod, root, this.parent, previous, drawn, nodes)

    // A node no parent reaches is not drawn; its panel goes with its old parent.
    for (const [id, old] of previous) {
      if (!drawn.has(id) && old.panel.IsValid()) {
        this.delete(old)
      }
    }
    this.mods.set(mod, drawn)
  }

  /** delete deletes a drawn node's panel and notes a layout's. */
  private delete(drawn: Drawn): void {
    drawn.panel.DeleteAsync(0)
    this.unloaded ||= drawn.layout !== null
  }

  /** drawNode draws node under parent, then its children in order. */
  private drawNode(
    mod: string,
    node: Node,
    parent: Panel,
    previous: Map<string, Drawn>,
    drawn: Map<string, Drawn>,
    nodes: Map<string, Node>,
  ): Panel {
    // Resolve the node's id, panel type, style, and the URL of its layout.
    const id = node.id ?? ''
    const kind = id === '' ? Kind.PANEL : (node.kind ?? Kind.PANEL)
    const type = panelTypes[kind] ?? 'Panel'
    const style = styleOf(node.style, id === '')
    const path = node.layout ?? ''
    const layout =
      kind === Kind.LAYOUT && isLayoutPath(path)
        ? (this.layouts?.url(mod, path) ?? null)
        : null
    const data = node.data ?? ''

    // Reuse the node's panel when its type, parent and layout stay and its
    // style keeps every property, since Panorama cannot unset some of them;
    // or make one.
    const old = previous.get(id)
    let panel = old?.panel
    let written = old?.style ?? {}
    let loaded = old?.loaded ?? false
    let given = old?.data ?? ''
    const kept =
      old &&
      old.parent === parent &&
      old.layout === layout &&
      Object.keys(written).every((name) => name in style)
    if (!panel || !kept || !panel.IsValid() || panel.paneltype !== type) {
      if (old?.panel.IsValid()) {
        this.delete(old)
      }
      panel = this.panorama.CreatePanel(
        type,
        parent,
        id === '' ? `ModlockUi_${mod.replace(/\W/g, '_')}` : 'ModlockUiNode',
      )
      written = {}
      loaded = false
      given = ''
      if (kind === Kind.BUTTON) {
        panel.SetPanelEvent('onactivate', () => this.press(mod, id))
      }
    }

    // Load the node's layout with its data, or hand a loaded layout its new
    // data. A layout whose addon has not mounted yet loads on a later draw.
    if (layout && !loaded) {
      panel.SetAttributeString(layoutDataAttribute, data)
      loaded = panel.BLoadLayout(layout, false, false)
      if (!loaded) {
        this.panorama.Msg(`[Modlock] ${mod} layout ${path} is unavailable`)
      }
    } else if (layout && data !== given) {
      panel.SetAttributeString(layoutDataAttribute, data)
      this.panorama.DispatchEvent('PlaySoundEffect', layoutDataChanged)
    }
    drawn.set(id, { panel, parent, style, layout, loaded, data })

    // Write the node's content and the style properties that changed. A
    // layout's own root decides whether it takes clicks.
    if (!layout) {
      panel.hittest = kind === Kind.BUTTON
    }
    if (kind === Kind.LABEL) {
      panel.text = node.text ?? ''
    } else if (kind === Kind.IMAGE) {
      panel.SetImage(node.image ?? '')
    }
    for (const [name, value] of Object.entries(style)) {
      if (written[name] !== value) {
        this.write(panel, name, value)
      }
    }

    // Draw each child once, in order; a child seen before would make a cycle.
    // A layout's panels come from its file, not from the tree.
    let last: Panel | undefined
    for (const child of kind === Kind.LAYOUT ? [] : (node.children ?? [])) {
      const childNode = nodes.get(child)
      if (!childNode || drawn.has(child) || child === '') {
        continue
      }
      const childPanel = this.drawNode(
        mod,
        childNode,
        panel,
        previous,
        drawn,
        nodes,
      )
      if (last) {
        panel.MoveChildAfter(childPanel, last)
      } else if (panel.GetChild(0) && panel.GetChild(0) !== childPanel) {
        panel.MoveChildBefore(childPanel, panel.GetChild(0)!)
      }
      last = childPanel
    }
    return panel
  }

  /** write sets one style property; a refused property leaves the rest drawn. */
  private write(panel: Panel, name: string, value: string): void {
    try {
      panel.style[name] = value
    } catch (error) {
      if (!refused.has(name)) {
        refused.add(name)
        this.panorama.Msg(
          `[Modlock] the game refused the style ${name}: ${String(error)}`,
        )
      }
    }
  }
}

/** aligns maps each alignment to its horizontal and vertical style values. */
const aligns: Record<Align, [string, string]> = {
  [Align.START]: ['left', 'top'],
  [Align.CENTER]: ['center', 'center'],
  [Align.END]: ['right', 'bottom'],
}

/**
 * faces maps each typeface to the game's font stack, as its base styles name
 * them: Retail Demo for text, VALVEPulp for the clock and titles, Reaver for
 * menus.
 */
const faces: Partial<Record<Face, string>> = {
  [Face.BODY]: '"Retail Demo", "Noto Sans", sans-serif',
  [Face.BLOCK]: 'VALVEPulp, "Noto Sans", sans-serif',
  [Face.SERIF]: 'Reaver, serif',
}

/**
 * textShadow is a soft drop under the glyphs rather than a halo that thickens
 * them, so text reads over a bright sky.
 */
const textShadow = '0px 1px 3px 1 #000000d0'

/** styleOf returns the style properties to write; a root fills the screen by default. */
function styleOf(
  style: Style | undefined,
  root: boolean,
): Record<string, string> {
  const css: Record<string, string | null> = {
    width: length(style?.width) ?? (root ? '100%' : null),
    height: length(style?.height) ?? (root ? '100%' : null),
    flowChildren:
      style?.flow === Flow.DOWN
        ? 'down'
        : style?.flow === Flow.RIGHT
          ? 'right'
          : null,
    horizontalAlign: style?.horizontalAlign
      ? aligns[style.horizontalAlign][0]
      : null,
    verticalAlign: style?.verticalAlign ? aligns[style.verticalAlign][1] : null,
    margin: edges(style?.margin),
    padding: edges(style?.padding),
    backgroundColor: style?.background ? color(style.background) : null,
    color: style?.color ? color(style.color) : null,
    fontSize: style?.fontSize ? `${finite(style.fontSize)}px` : null,
    fontWeight: style?.bold ? 'bold' : null,
    textAlign: style?.textAlign ? aligns[style.textAlign][0] : null,
    borderRadius: style?.borderRadius
      ? `${finite(style.borderRadius)}px`
      : null,
    opacity:
      style?.opacity === undefined ? null : String(clamp(style.opacity, 0, 1)),
    fontFamily: style?.face ? (faces[style.face] ?? null) : null,
    textTransform: style?.uppercase ? 'uppercase' : null,
    letterSpacing: style?.letterSpacing
      ? `${finite(style.letterSpacing)}px`
      : null,
    textShadow: style?.shadow ? textShadow : null,
    border:
      style?.borderWidth && style.borderColor
        ? `${finite(style.borderWidth)}px solid ${color(style.borderColor)}`
        : null,
  }
  return Object.fromEntries(
    Object.entries(css).filter(
      (entry): entry is [string, string] => entry[1] !== null,
    ),
  )
}

/** refused holds the style properties the game refused, each logged once. */
const refused = new Set<string>()

/** length writes a length, or null for none. */
function length(value: Length | undefined): string | null {
  switch (value?.value?.case) {
    case 'pixels':
      return `${finite(value.value.value)}px`
    case 'percent':
      return `${clamp(value.value.value, 0, 100)}%`
    case 'fitChildren':
      return 'fit-children'
    case 'fill':
      return `fill-parent-flow(${clamp(value.value.value, 0, 100)})`
    default:
      return null
  }
}

/** edges writes four distances, or null for none. */
function edges(value: Edges | undefined): string | null {
  if (!value) {
    return null
  }
  return `${finite(value.top)}px ${finite(value.right)}px ${finite(value.bottom)}px ${finite(value.left)}px`
}

/** color writes 0xRRGGBBAA as rgba(). */
function color(value: number): string {
  return `rgba(${(value >>> 24) & 255}, ${(value >>> 16) & 255}, ${(value >>> 8) & 255}, ${((value & 255) / 255).toFixed(3)})`
}

/** clamp keeps value within [low, high]. */
function clamp(value: number, low: number, high: number): number {
  return Math.min(high, Math.max(low, finite(value)))
}

/** finite returns value, or zero when it is unset or not a finite number. */
function finite(value: number | undefined): number {
  return value !== undefined && Number.isFinite(value) ? value : 0
}
