package domain

// ActionCost returns the credit cost for a given action type.
func ActionCost(actionType string) int {
	costs := map[string]int{
		"navigate":   5,
		"click":      1,
		"type":       1,
		"extract":    3,
		"wait":       1,
		"scroll":     1,
		"hover":      1,
		"select":     1,
		"screenshot": 10,
		"upload":     2,
		"pause":      0,
		"new_tab":    1,
		"switch_tab": 1,
	}
	if c, ok := costs[actionType]; ok {
		return c
	}
	return 1 // default
}
