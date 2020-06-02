package terraform

type Module struct {
	Arguments map[string]Value
	Label     Value
	Provider  ProviderAlias
	Source    Value
}

func (m Module) Ref() Value {
	return Uf("module.%s", m.Label)
}

func (Module) Template() string {
	return `module {{.Label.Value}} {
{{- range $k, $v := .Arguments }}
	{{$k}} = {{$v.Value}}
{{- end}}

	providers = {
		aws = {{.Provider}}
	}
	source = {{.Source.Value}}
}`
}
