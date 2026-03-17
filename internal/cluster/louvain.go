package cluster

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yourusername/mimir/internal/graph"
)

// Graph is the undirected weighted graph for Louvain clustering.
type Graph struct {
	Nodes       []string       // node UIDs
	NodeIndex   map[string]int // uid → index
	Edges       [][]WEdge      // adjacency list
	Degree      []float64      // weighted degree per node
	TotalWeight float64        // sum of all edge weights (already halved for undirected)
}

// WEdge is a weighted edge in the adjacency list.
type WEdge struct {
	To     int
	Weight float64
}

// Community is a group of closely related nodes.
type Community struct {
	ID            string
	Members       []string // node UIDs
	CohesionScore float64
	Label         string
}

// BuildGraph constructs a Louvain graph from nodes and edges.
// Only CALLS and IMPORTS edges are used (weighted by Confidence).
func BuildGraph(nodes []graph.Node, edges []graph.Edge) *Graph {
	g := &Graph{NodeIndex: make(map[string]int)}
	for i, n := range nodes {
		g.Nodes = append(g.Nodes, n.UID)
		g.NodeIndex[n.UID] = i
		g.Edges = append(g.Edges, nil)
		g.Degree = append(g.Degree, 0)
	}

	for _, e := range edges {
		if e.Type != graph.EdgeCalls && e.Type != graph.EdgeImports {
			continue
		}
		fi, ok1 := g.NodeIndex[e.FromUID]
		ti, ok2 := g.NodeIndex[e.ToUID]
		if !ok1 || !ok2 {
			continue
		}
		w := e.Confidence
		g.Edges[fi] = append(g.Edges[fi], WEdge{ti, w})
		g.Edges[ti] = append(g.Edges[ti], WEdge{fi, w})
		g.Degree[fi] += w
		g.Degree[ti] += w
		g.TotalWeight += w
	}
	return g
}

// Louvain runs the Louvain community detection algorithm.
func Louvain(g *Graph, resolution float64) []Community {
	if len(g.Nodes) == 0 {
		return nil
	}

	comm := make([]int, len(g.Nodes))
	for i := range comm {
		comm[i] = i
	}

	// Map original index → current super-node index across aggregations
	nodeToSuper := make([]int, len(g.Nodes))
	for i := range nodeToSuper {
		nodeToSuper[i] = i
	}

	currentG := g
	for {
		improved := phase1(currentG, comm, resolution)
		if !improved {
			break
		}
		newG, newComm := phase2(currentG, comm)
		for i := range nodeToSuper {
			nodeToSuper[i] = newComm[nodeToSuper[i]]
		}
		currentG = newG
		comm = make([]int, len(newG.Nodes))
		for i := range comm {
			comm[i] = i
		}
	}

	// Group original nodes by final community
	groups := map[int][]int{}
	for origIdx, superIdx := range nodeToSuper {
		groups[superIdx] = append(groups[superIdx], origIdx)
	}

	communities := make([]Community, 0, len(groups))
	for cID, members := range groups {
		uids := make([]string, len(members))
		for i, m := range members {
			uids[i] = g.Nodes[m]
		}
		communities = append(communities, Community{
			ID:            fmt.Sprintf("cluster_%d", cID),
			Members:       uids,
			CohesionScore: cohesion(g, members),
		})
	}
	return communities
}

func phase1(g *Graph, comm []int, resolution float64) bool {
	if g.TotalWeight == 0 {
		return false
	}
	improved := false
	commWeight := make([]float64, len(g.Nodes))
	for i, c := range comm {
		commWeight[c] += g.Degree[i]
	}

	m2 := 2 * g.TotalWeight

	for i := range g.Nodes {
		bestComm := comm[i]
		bestGain := 0.0

		neighComms := map[int]float64{}
		for _, e := range g.Edges[i] {
			if e.To != i {
				neighComms[comm[e.To]] += e.Weight
			}
		}

		ki := g.Degree[i]
		commWeight[comm[i]] -= ki

		for c, kIn := range neighComms {
			gain := resolution*(kIn/g.TotalWeight) -
				(commWeight[c]*ki)/(m2*g.TotalWeight)
			if gain > bestGain {
				bestGain = gain
				bestComm = c
			}
		}

		if bestComm != comm[i] {
			comm[i] = bestComm
			improved = true
		}
		commWeight[comm[i]] += ki
	}
	return improved
}

func phase2(g *Graph, comm []int) (*Graph, []int) {
	remap := map[int]int{}
	k := 0
	for _, c := range comm {
		if _, seen := remap[c]; !seen {
			remap[c] = k
			k++
		}
	}
	newComm := make([]int, len(comm))
	for i, c := range comm {
		newComm[i] = remap[c]
	}

	newG := &Graph{
		Nodes:     make([]string, k),
		NodeIndex: make(map[string]int),
		Edges:     make([][]WEdge, k),
		Degree:    make([]float64, k),
	}
	for i := range newG.Nodes {
		newG.Nodes[i] = fmt.Sprintf("super_%d", i)
		newG.NodeIndex[newG.Nodes[i]] = i
	}

	edgeAcc := map[[2]int]float64{}
	for i := range g.Nodes {
		for _, e := range g.Edges[i] {
			ci, cj := newComm[i], newComm[e.To]
			key := [2]int{min(ci, cj), max(ci, cj)}
			edgeAcc[key] += e.Weight
		}
	}
	for key, w := range edgeAcc {
		ci, cj := key[0], key[1]
		newG.Edges[ci] = append(newG.Edges[ci], WEdge{cj, w})
		if ci != cj {
			newG.Edges[cj] = append(newG.Edges[cj], WEdge{ci, w})
		}
		newG.Degree[ci] += w
		newG.Degree[cj] += w
		newG.TotalWeight += w
	}

	return newG, newComm
}

func cohesion(g *Graph, members []int) float64 {
	set := make(map[int]bool, len(members))
	for _, m := range members {
		set[m] = true
	}
	var intra, total float64
	for _, m := range members {
		for _, e := range g.Edges[m] {
			total += e.Weight
			if set[e.To] {
				intra += e.Weight
			}
		}
	}
	if total == 0 {
		return 0
	}
	return intra / total
}

// LabelCommunity picks a human-readable label using most common directory prefix.
func LabelCommunity(members []graph.Node) string {
	dirCount := map[string]int{}
	for _, n := range members {
		dir := filepath.Dir(n.FilePath)
		parts := strings.Split(dir, string(os.PathSeparator))
		if len(parts) >= 2 {
			label := strings.Join(parts[len(parts)-2:], "/")
			dirCount[label]++
		} else {
			dirCount[dir]++
		}
	}
	return topKey(dirCount)
}

func topKey(m map[string]int) string {
	best, bestCount := "", 0
	for k, v := range m {
		if v > bestCount {
			best, bestCount = k, v
		}
	}
	return best
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
