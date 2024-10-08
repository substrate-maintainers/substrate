package terraform

type EgressOnlyInternetGateway struct {
	Label    Value
	Provider ProviderAlias
	Tags     Tags
	VpcId    Value
}

func (egw EgressOnlyInternetGateway) Ref() Value {
	return Uf("aws_egress_only_internet_gateway.%s", egw.Label)
}

func (EgressOnlyInternetGateway) Template() string {
	return `resource "aws_egress_only_internet_gateway" {{.Label.Value}} {
{{- if .Provider}}
  provider = {{.Provider}}
{{- end}}
  tags = {{.Tags.Value}}
  vpc_id = {{.VpcId.Value}}
}`
}

type InternetGateway struct {
	Label    Value
	Provider ProviderAlias
	Tags     Tags
	VpcId    Value
}

func (igw InternetGateway) Ref() Value {
	return Uf("aws_internet_gateway.%s", igw.Label)
}

func (InternetGateway) Template() string {
	return `resource "aws_internet_gateway" {{.Label.Value}} {
{{- if .Provider}}
  provider = {{.Provider}}
{{- end}}
  tags = {{.Tags.Value}}
  vpc_id = {{.VpcId.Value}}
}`
}

type NATGateway struct {
	Commented          bool // set by a command-line flag to control costs
	InternetGatewayRef Value
	Label              Value
	Provider           ProviderAlias
	SubnetId           Value
	Tags               Tags
}

func (ngw NATGateway) Ref() Value {
	return Uf("aws_nat_gateway.%s", ngw.Label)
}

func (NATGateway) Template() string {
	return `{{if .Commented -}}
/* commented because substrate.nat-gateways contains "no"
{{end -}}
resource "aws_nat_gateway" {{.Label.Value}} {
  allocation_id = aws_eip.{{.Label}}.id
{{- if .Provider}}
  provider = {{.Provider}}
{{- end}}
  subnet_id = {{.SubnetId.Value}}
  tags = {{.Tags.Value}}
}
{{- if .Commented}}
*/
{{- end}}`
}
