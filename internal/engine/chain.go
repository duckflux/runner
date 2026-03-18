package engine

import "fmt"

// mergeChainedInput merges the implicit chain value with an explicit input mapping
// according to the v0.3 spec merge rules:
//   - If explicit is nil, return chain as-is.
//   - If chain is nil, return explicit as-is.
//   - map + map: merge keys, explicit wins on conflict.
//   - string + string: explicit wins.
//   - incompatible types: runtime error.
func mergeChainedInput(chain any, explicit any) (any, error) {
	if explicit == nil {
		return chain, nil
	}
	if chain == nil {
		return explicit, nil
	}

	chainMap, chainIsMap := toStringMap(chain)
	explicitMap, explicitIsMap := toStringMap(explicit)

	if chainIsMap && explicitIsMap {
		// map + map: merge keys, explicit wins on key conflict.
		merged := make(map[string]any, len(chainMap)+len(explicitMap))
		for k, v := range chainMap {
			merged[k] = v
		}
		for k, v := range explicitMap {
			merged[k] = v
		}
		return merged, nil
	}

	// Incompatible types: one is a map and the other is not — runtime error per spec §5.7.
	if chainIsMap != explicitIsMap {
		return nil, fmt.Errorf("chain merge: incompatible types (chain is %T, explicit is %T)", chain, explicit)
	}

	// Same non-map types (string + string, scalar + scalar): explicit wins.
	return explicit, nil
}

// toStringMap attempts to convert a value to map[string]any.
func toStringMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}
