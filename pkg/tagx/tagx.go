package tagx

func Copy(raw map[string]string) map[string]string {
	ret := make(map[string]string)
	for k, v := range raw {
		ret[k] = v
	}
	return ret
}
