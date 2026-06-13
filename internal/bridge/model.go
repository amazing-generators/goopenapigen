package bridge

// // // // // // // // // //

type ConfigObj struct {
	OgenDir          string
	PackageName      string
	Document         map[string]any
	RequireXFunc     bool
	Comments         bool
	OpenAPIJSON      []byte
	HasOpenAPIJSON   bool
	ServerOptionName string
}

type FieldObj struct {
	Name  string
	Type  string
	Named bool
}

type StructFieldObj struct {
	Name string
	Type string
}

type MethodObj struct {
	Name      string
	ParamArr  []FieldObj
	ResultArr []FieldObj
}

type ParseResultObj struct {
	HandlerInterfaceName  string
	SecurityInterfaceName string
	ServerOptionName      string
	HandlerMethodArr      []MethodObj
	SecurityMethodArr     []MethodObj
	ImportMap             map[string]string
	StructFieldMap        map[string]map[string]StructFieldObj
}

type MappingObj struct {
	OperationFuncMap map[string]string
	SecurityFuncMap  map[string]string
	FuncDocMap       map[string]funcDocObj
	MissingArr       []string
}

// funcDocObj is the source of the doc comment for a FuncInterface method.
type funcDocObj struct {
	Security bool
	Method   string
	Path     string
	Scheme   string
	Summary  string
	Desc     string
}

type RenderMethodObj struct {
	MethodObj MethodObj
	FuncName  string
	UseCall   bool
	NeedError bool
}
