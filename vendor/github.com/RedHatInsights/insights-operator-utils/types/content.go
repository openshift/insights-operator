package types

// RuleContent wraps all the content available for a rule into a single structure.
type RuleContent struct {
	Summary    string                         `json:"summary"`
	Reason     string                         `json:"reason"`
	Resolution string                         `json:"resolution"`
	MoreInfo   string                         `json:"more_info"`
	Plugin     RulePluginInfo                 `json:"plugin"`
	ErrorKeys  map[string]RuleErrorKeyContent `json:"error_keys"`
	HasReason  bool
}

// RulePluginInfo is a Go representation of the `plugin.yaml`
// file inside of the rule content directory.
type RulePluginInfo struct {
	Name         string `yaml:"name" json:"name"`
	NodeID       string `yaml:"node_id" json:"node_id"`
	ProductCode  string `yaml:"product_code" json:"product_code"`
	PythonModule string `yaml:"python_module" json:"python_module"`
}

// RuleErrorKeyContent wraps content of a single error key.
type RuleErrorKeyContent struct {
	Generic   string           `json:"generic"`
	Metadata  ErrorKeyMetadata `json:"metadata"`
	Reason    string           `json:"reason"`
	HasReason bool
}

// ErrorKeyMetadata is a Go representation of the `metadata.yaml`
// file inside of an error key content directory.
type ErrorKeyMetadata struct {
	Condition   string   `yaml:"condition" json:"condition"`
	Description string   `yaml:"description" json:"description"`
	Impact      string   `yaml:"impact" json:"impact"`
	Likelihood  int      `yaml:"likelihood" json:"likelihood"`
	PublishDate string   `yaml:"publish_date" json:"publish_date"`
	Status      string   `yaml:"status" json:"status"`
	Tags        []string `yaml:"tags" json:"tags"`
}

// RuleContentDirectory contains content for all available rules in a directory.
type RuleContentDirectory struct {
	Config GlobalRuleConfig
	Rules  map[string]RuleContent
}

// GlobalRuleConfig represents the file that contains
// metadata globally applicable to any/all rule content.
type GlobalRuleConfig struct {
	Impact map[string]int `yaml:"impact" json:"impact"`
}
