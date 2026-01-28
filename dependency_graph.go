package main

import (
	"fmt"
	"sort"
)

type DependencyGraph struct {
	graph    map[string]map[string]bool
	roots    map[string]bool
	contains map[string]map[string]bool // tracks which functions contain/define other functions
}

func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{
		graph:    make(map[string]map[string]bool),
		roots:    make(map[string]bool),
		contains: make(map[string]map[string]bool),
	}
}

func (dg *DependencyGraph) AddCall(caller, callee string) {
	if dg.graph[caller] == nil {
		dg.graph[caller] = make(map[string]bool)
	}
	dg.graph[caller][callee] = true
}

func (dg *DependencyGraph) AddContains(parent, child string) {
	if dg.contains[parent] == nil {
		dg.contains[parent] = make(map[string]bool)
	}
	dg.contains[parent][child] = true
}

func (dg *DependencyGraph) MarkRoot(funcName string) {
	dg.roots[funcName] = true
}

func (dg *DependencyGraph) GetReachable() map[string]bool {
	reachable := make(map[string]bool)
	visited := make(map[string]bool)

	var dfs func(string)
	dfs = func(funcName string) {
		if visited[funcName] {
			return
		}
		visited[funcName] = true
		reachable[funcName] = true

		// Follow direct calls
		for callee := range dg.graph[funcName] {
			dfs(callee)
		}

		// Follow contained functions (nested lambdas)
		for child := range dg.contains[funcName] {
			dfs(child)
		}
	}

	for root := range dg.roots {
		dfs(root)
	}

	return reachable
}

func (dg *DependencyGraph) PrintDependencyTree() {
	fmt.Println("=== Dependency Tree ===")
	fmt.Println()

	fmt.Println("Entry Points:")
	for root := range dg.roots {
		fmt.Printf("  - %s\n", root)
	}
	fmt.Println()

	reachable := dg.GetReachable()
	unreachable := make(map[string]bool)

	for funcName := range dg.graph {
		if !reachable[funcName] {
			unreachable[funcName] = true
		}
	}

	fmt.Printf("Reachable Functions: %d\n", len(reachable))
	funcs := make([]string, 0, len(reachable))
	for fn := range reachable {
		funcs = append(funcs, fn)
	}
	sort.Strings(funcs)
	for _, fn := range funcs {
		callees := dg.graph[fn]
		if len(callees) > 0 {
			calleeList := make([]string, 0, len(callees))
			for c := range callees {
				calleeList = append(calleeList, c)
			}
			sort.Strings(calleeList)
			fmt.Printf("  %s -> %v\n", fn, calleeList)
		} else {
			fmt.Printf("  %s (leaf)\n", fn)
		}
	}
	fmt.Println()

	if len(unreachable) > 0 {
		fmt.Printf("Dead Code (Eliminated): %d functions\n", len(unreachable))
		deadFuncs := make([]string, 0, len(unreachable))
		for fn := range unreachable {
			deadFuncs = append(deadFuncs, fn)
		}
		sort.Strings(deadFuncs)
		for _, fn := range deadFuncs {
			fmt.Printf("  - %s\n", fn)
		}
		fmt.Println()
	}
}
