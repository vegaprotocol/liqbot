package types

type Meta struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Key struct {
	Pub     string `json:"pub"`
	Algo    string `json:"algo"`
	Tainted bool   `json:"tainted"`
	Meta    []Meta `json:"meta"`
}
