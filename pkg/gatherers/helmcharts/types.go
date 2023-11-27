package helmcharts

type HelmChartInfo struct {
	Name      string         `json:"name"`
	Version   string         `json:"version"`
	Resources map[string]int `json:"resources"`
}

type HelmChartInfoList struct {
	Namespaces map[string][]HelmChartInfo
}

func newHelmChartInfoList() HelmChartInfoList {
	return HelmChartInfoList{
		Namespaces: make(map[string][]HelmChartInfo),
	}
}

func (h *HelmChartInfoList) addItem(ns string, resourceType string, info HelmChartInfo) {
	if _, ok := h.Namespaces[ns]; !ok {
		h.Namespaces[ns] = make([]HelmChartInfo, 0)
	}

	var helmIdx int
	var found bool
	for i, n := range h.Namespaces[ns] {
		if n.Name == info.Name && n.Version == info.Version {
			helmIdx = i
			found = true
			break
		}
	}

	if !found {
		info.Resources = map[string]int{resourceType: 1}
		h.Namespaces[ns] = append(h.Namespaces[ns], info)
		return
	}

	if h.Namespaces[ns][helmIdx].Resources == nil {
		h.Namespaces[ns][helmIdx].Resources = make(map[string]int)
	}
	h.Namespaces[ns][helmIdx].Resources[resourceType]++
}
