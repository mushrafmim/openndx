package graphql

type ArgMapping struct {
	TargetArgName string `json:"targetArgName" yaml:"targetArgName"`
	SchemaID      string `json:"schemaId" yaml:"schemaId"`
	SourceArgPath string `json:"sourceArgPath" yaml:"sourceArgPath"`
	TargetArgPath string `json:"targetArgPath" yaml:"targetArgPath"`
}
