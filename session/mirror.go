package session

import (
	"sync"

	"github.com/paralin/modlock/src/wasm/ui"
)

// Mirror keeps a copy of one player's screen that another PC sends as
// changes, for a Relay on the player's own PC.
type Mirror struct {
	// mu guards the fields below.
	mu sync.Mutex
	// trees is the player's screen.
	trees trees
	// changed is closed and replaced whenever the screen changes.
	changed chan struct{}
}

// NewMirror returns an empty screen.
func NewMirror() *Mirror {
	return &Mirror{trees: make(trees), changed: make(chan struct{})}
}

// Apply applies each mod's change to the screen, first clearing it on reset,
// when changes hold the whole screen.
func (m *Mirror) Apply(changes map[string]*ui.Change, reset bool) {
	// Change the trees, then wake every stream.
	m.mu.Lock()
	defer m.mu.Unlock()
	if reset {
		m.trees = make(trees)
	}
	for mod, change := range changes {
		m.trees.apply(mod, change)
	}
	close(m.changed)
	m.changed = make(chan struct{})
}

// Screen returns the screen and a channel that closes when it changes.
func (m *Mirror) Screen() (*ui.Screen, <-chan struct{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.trees.screen(), m.changed
}

// Changes returns each mod's change that turns the screen from into to: the
// nodes set or removed in a tree both hold, a whole new tree as a reset, and
// a reset with no nodes for a tree that is gone. It returns nothing when the
// screens match.
func Changes(from, to *ui.Screen) map[string]*ui.Change {
	// Index the old trees' nodes by id.
	old := make(map[string]map[string]*ui.Node, len(from.GetTrees()))
	for _, tree := range from.GetTrees() {
		nodes := make(map[string]*ui.Node, len(tree.GetNodes()))
		for _, node := range tree.GetNodes() {
			nodes[node.GetId()] = node
		}
		old[tree.GetMod()] = nodes
	}

	// Change each new tree, and clear each one that is gone.
	changes := make(map[string]*ui.Change)
	for _, tree := range to.GetTrees() {
		if change := treeChange(old[tree.GetMod()], tree.GetNodes()); change != nil {
			changes[tree.GetMod()] = change
		}
		delete(old, tree.GetMod())
	}
	for mod := range old {
		changes[mod] = &ui.Change{Reset_: true}
	}
	return changes
}

// treeChange returns the change that turns a tree with the nodes old into one
// with nodes, or nil when they match. A tree absent before is sent whole.
func treeChange(old map[string]*ui.Node, nodes []*ui.Node) *ui.Change {
	// Send a new tree whole.
	if old == nil {
		return &ui.Change{Reset_: true, Set: nodes}
	}

	// Set the nodes that differ, and remove those that are gone.
	change := &ui.Change{}
	for _, node := range nodes {
		if !node.EqualVT(old[node.GetId()]) {
			change.Set = append(change.Set, node)
		}
		delete(old, node.GetId())
	}
	for id := range old {
		change.Removed = append(change.Removed, id)
	}
	if len(change.Set) == 0 && len(change.Removed) == 0 {
		return nil
	}
	return change
}
