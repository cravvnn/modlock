import type { Panel } from '../src/draw.js'

/**
 * DomPanel stands in for a Panorama panel with one HTML element, so a mod's
 * interface renders in a browser for layout work. It approximates Panorama
 * layout with the CSS in panels.css, which the page must link: children
 * overlap unless the panel flows them down or right, and fill-parent-flow
 * takes the remaining space. Fonts and engine-only styles are approximate,
 * and it loads no layout files; the installed game remains the reference.
 */
export class DomPanel implements Panel {
  readonly element: HTMLElement
  readonly style: Record<string, string>
  private readonly handlers = new Map<string, (...args: unknown[]) => void>()
  private readonly attributes = new Map<string, string>()
  private valid = true
  private accepts = false

  constructor(
    readonly paneltype: string,
    readonly id: string,
    host?: HTMLElement,
  ) {
    // Mount the element with its panel classes and a back reference.
    this.element = host ?? document.createElement('div')
    this.element.classList.add('p', `p-${paneltype}`, 'flow-none')
    if (id) this.element.dataset.id = id
    ;(this.element as HTMLElement & { panel?: DomPanel }).panel = this

    // Engine style writes arrive one property at a time.
    const element = this.element
    this.style = new Proxy<Record<string, string>>(
      {},
      {
        set(target, name: string, value: string) {
          target[name] = value
          applyStyle(element, name, value)
          return true
        },
        deleteProperty(target, name: string) {
          delete target[name]
          applyStyle(element, name, '')
          return true
        },
      },
    )

    // Native activation, hover and focus events map to their DOM equivalents.
    this.element.addEventListener('click', (event) => {
      if (!this.enabled) return
      event.stopPropagation()
      this.handlers.get('onactivate')?.()
    })
    this.element.addEventListener('mouseenter', () =>
      this.handlers.get('onmouseover')?.(),
    )
    this.element.addEventListener('mouseleave', () =>
      this.handlers.get('onmouseout')?.(),
    )
    this.element.addEventListener('focus', () =>
      this.handlers.get('onfocus')?.(),
    )
    this.element.addEventListener('blur', () => this.handlers.get('onblur')?.())
  }

  get text(): string {
    return this.element.textContent ?? ''
  }
  set text(value: string) {
    this.element.textContent = value
    shrink(this.element)
  }

  get visible(): boolean {
    return !this.element.classList.contains('hidden')
  }
  set visible(value: boolean) {
    this.element.classList.toggle('hidden', !value)
  }

  get hittest(): boolean {
    return !this.element.classList.contains('no-hit')
  }
  set hittest(value: boolean) {
    this.element.classList.toggle('no-hit', !value)
  }

  get hittestchildren(): boolean {
    return !this.element.classList.contains('no-hit-children')
  }
  set hittestchildren(value: boolean) {
    this.element.classList.toggle('no-hit-children', !value)
  }

  get enabled(): boolean {
    return !this.element.classList.contains('disabled')
  }
  set enabled(value: boolean) {
    this.element.classList.toggle('disabled', !value)
  }

  IsValid() {
    return this.valid
  }

  DeleteAsync() {
    this.valid = false
    this.element.remove()
  }

  SetAcceptsFocus(accepts: boolean) {
    this.accepts = accepts
    if (accepts) this.element.tabIndex = 0
    else this.element.removeAttribute('tabindex')
  }

  SetFocus() {
    if (!this.accepts) return false
    this.element.focus()
    return true
  }

  BHasKeyFocus() {
    return document.activeElement === this.element
  }

  GetParent(): DomPanel | null {
    return panelOf(this.element.parentElement)
  }

  GetChildCount() {
    return this.element.children.length
  }

  GetChild(index: number): DomPanel | null {
    return panelOf(this.element.children[index])
  }

  FindChildTraverse(id: string): DomPanel | null {
    return panelOf(this.element.querySelector(`[data-id="${CSS.escape(id)}"]`))
  }

  ScrollToBottom() {
    this.element.scrollTop = this.element.scrollHeight
  }

  SetParent(parent: DomPanel) {
    parent.element.append(this.element)
  }

  MoveChildBefore(child: DomPanel, before: DomPanel) {
    this.element.insertBefore(child.element, before.element)
  }

  MoveChildAfter(child: DomPanel, after: DomPanel) {
    after.element.after(child.element)
  }

  AddClass(name: string) {
    this.element.classList.add(name)
  }

  RemoveClass(name: string) {
    this.element.classList.remove(name)
  }

  SetPanelEvent(event: string, handler: (...args: unknown[]) => void) {
    this.handlers.set(event, handler)
  }

  ClearPropertyFromCode(name: string) {
    delete this.style[
      name.replace(/-([a-z])/g, (_, letter: string) => letter.toUpperCase())
    ]
  }

  SetAttributeString(name: string, value: string) {
    this.attributes.set(name, value)
  }

  GetAttributeString(name: string, fallback: string) {
    return this.attributes.get(name) ?? fallback
  }

  SetImage(path: string) {
    this.element.style.backgroundImage = path
      ? `url(${JSON.stringify(path)})`
      : ''
  }

  BLoadLayout() {
    return false
  }
}

// panelOf returns the stand-in panel an element belongs to, if any.
function panelOf(element: Element | null | undefined): DomPanel | null {
  return (element as (Element & { panel?: DomPanel }) | null)?.panel ?? null
}

// applyStyle translates one Panorama style property, named in either case
// the engine accepts, into CSS.
function applyStyle(element: HTMLElement, kebab: string, value: string): void {
  const css = element.style
  const name = kebab.replace(/-([a-z])/g, (_, letter: string) =>
    letter.toUpperCase(),
  )
  switch (name) {
    case 'flowChildren': {
      element.classList.remove(
        'flow-none',
        'flow-down',
        'flow-right',
        'flow-right-wrap',
        'flow-down-wrap',
      )
      element.classList.add(`flow-${value || 'none'}`)
      return
    }
    case 'horizontalAlign':
    case 'verticalAlign': {
      const key = name === 'horizontalAlign' ? 'ha' : 'va'
      if (value) element.dataset[key] = value
      else delete element.dataset[key]
      return
    }
    case 'width':
    case 'height': {
      const grow = /^fill-parent-flow\(\s*([\d.]+)\s*\)$/.exec(value)
      element.classList.toggle(`grow-${name}`, !!grow)
      if (grow) {
        css.setProperty('--grow', grow[1] ?? '1')
        css.removeProperty(name)
      } else {
        css.setProperty(name, value === 'fit-children' ? '' : value)
      }
      return
    }
    case 'overflow': {
      const [x = '', y = x] = value.split(/\s+/)
      const map = (part: string) =>
        part === 'scroll' ? 'auto' : part === 'noclip' ? 'visible' : 'hidden'
      css.overflowX = value ? map(x) : ''
      css.overflowY = value ? map(y) : ''
      return
    }
    case 'textShadow': {
      // Panorama adds a strength term after the blur radius.
      css.textShadow = value.replace(/^(\S+ \S+ \S+) [\d.]+ (\S+)$/, '$1 $2')
      return
    }
    case 'textOverflow': {
      element.classList.toggle('ellipsis', value === 'ellipsis')
      element.classList.toggle('shrink', value === 'shrink')
      shrink(element)
      return
    }
    case 'fontSize': {
      css.fontSize = value
      element.dataset.fontSize = value
      shrink(element)
      return
    }
    case 'brightness': {
      css.filter = value ? `brightness(${value})` : ''
      return
    }
    case 'backgroundColor': {
      css.background = gradient(value)
      return
    }
    case 'washColor':
    case 'saturation':
    case 'blur':
    case 'soundOut':
      return
    default:
      css.setProperty(
        name.replace(/[A-Z]/g, (letter) => `-${letter.toLowerCase()}`),
        value,
      )
  }
}

// shrink fits a label set to text-overflow: shrink, as Panorama does, by
// stepping its font size down from the declared one until the text fits its
// box. It waits a frame so the label is laid out at its final width.
const shrinking = new Set<HTMLElement>()
function shrink(element: HTMLElement): void {
  if (!element.classList.contains('shrink') || shrinking.has(element)) return
  shrinking.add(element)
  requestAnimationFrame(() => {
    // Start from the declared size and step down until the text fits.
    shrinking.delete(element)
    element.style.fontSize = element.dataset.fontSize ?? ''
    const fits = () =>
      element.scrollWidth <= element.clientWidth &&
      element.scrollHeight <= element.clientHeight
    let size = parseFloat(getComputedStyle(element).fontSize)
    while (!fits() && size > 6) element.style.fontSize = `${--size}px`
  })
}

// gradient converts Panorama's linear and radial gradient() syntax; plain
// colors pass through.
function gradient(value: string): string {
  // Parse a Panorama linear gradient: two endpoints, then the stops.
  const linear =
    /^gradient\(\s*linear,\s*(\S+)\s+(\S+),\s*(\S+)\s+(\S+),\s*(.*)\)$/.exec(
      value,
    )
  if (linear) {
    // Map its endpoints to a CSS angle.
    const [, x1 = '', y1 = '', x2 = '', y2 = '', stops = ''] = linear
    const angle =
      x1 === x2 ? (y1 < y2 ? '180deg' : '0deg') : x1 < x2 ? '90deg' : '270deg'
    return `linear-gradient(${angle}, ${colorStops(stops)})`
  }

  // Parse a Panorama radial gradient: its center, an offset the preview
  // ignores, its radii, then the stops.
  const radial =
    /^gradient\(\s*radial,\s*(\S+)\s+(\S+),\s*\S+\s+\S+,\s*(\S+)\s+(\S+),\s*(.*)\)$/.exec(
      value,
    )
  if (!radial) return value
  const [, x, y, rx, ry, stops = ''] = radial
  return `radial-gradient(ellipse ${rx} ${ry} at ${x} ${y}, ${colorStops(stops)})`
}

// colorStops maps Panorama's from, color-stop and to stops to CSS color stops.
function colorStops(stops: string): string {
  return [
    ...stops.matchAll(
      /(from|to|color-stop)\(\s*(?:([\d.]+)\s*,\s*)?([^)]+)\)/g,
    ),
  ]
    .map(([, kind, at, color = '']) =>
      kind === 'from'
        ? `${color.trim()} 0%`
        : kind === 'to'
          ? `${color.trim()} 100%`
          : `${color.trim()} ${Number(at) * 100}%`,
    )
    .join(', ')
}
