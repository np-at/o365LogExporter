package promtail

import "fmt"

type m = map[string]interface{}

// Given two maps, recursively merge right into left, NEVER replacing any key that already exists in left
func mergeKeys_string(left, right map[string]string) (map[string]string, error) {
	for key, rightVal := range right {
		if leftVal, present := left[key]; present {
			//then we don't want to replace it - recurse
			return nil, fmt.Errorf("found overlapping key for %v", leftVal)
			//left[key] = mergeKeys_string(leftVal, rightVal)
		} else {
			// key not in left so we can just shove it in
			left[key] = rightVal
		}
	}
	return left, nil
}
func mergeKeys(left, right m) m {
	for key, rightVal := range right {
		if leftVal, present := left[key]; present {
			//then we don't want to replace it - recurse
			left[key] = mergeKeys(leftVal.(m), rightVal.(m))
		} else {
			// key not in left so we can just shove it in
			left[key] = rightVal
		}
	}
	return left
}
func MergeJSONMaps(maps ...map[string]interface{}) (result map[string]interface{}) {
	result = make(map[string]interface{})
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}
