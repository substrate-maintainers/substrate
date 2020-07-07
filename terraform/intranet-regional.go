package terraform

// managed by go generate; do not edit by hand

func intranetRegionalTemplate() map[string]string {
	return map[string]string{
		".gitignore":   `*.zip
`,
		"variables.tf": `variable "dns_domain_name" {
  type = string
}

variable "oauth_oidc_client_id" {
  type = string
}

variable "oauth_oidc_client_secret_timestamp" {
  type = string
}

variable "okta_hostname" {
  default = ""
  type    = string
}

variable "selected_regions" {
  type = list(string)
}

variable "stage_name" {
  type = string
}

variable "validation_fqdn" {
  type = string
}
`,
		"outputs.tf":   `
`,
		"main.tf":      `data "aws_caller_identity" "current" {}

data "aws_iam_role" "apigateway" {
  name = "IntranetAPIGateway"
}

data "aws_iam_role" "substrate-apigateway-authenticator" {
  name = "substrate-apigateway-authenticator"
}

data "aws_iam_role" "substrate-apigateway-authorizer" {
  name = "substrate-apigateway-authorizer"
}

data "aws_iam_role" "substrate-credential-factory" {
  name = "substrate-credential-factory"
}

data "aws_iam_role" "substrate-instance-factory" {
  name = "substrate-instance-factory"
}

data "aws_region" "current" {}

data "aws_route53_zone" "intranet" {
  name         = "${var.dns_domain_name}."
  private_zone = false
}

locals {
  response_parameters = {
    "gatewayresponse.header.Location" = "'https://${var.dns_domain_name}/login?next=/credential-factory'" # a last resort
    #"gatewayresponse.header.Location"                  = "context.authorizer.Location" # use the authorizer for expensive string concatenation
    "gatewayresponse.header.Strict-Transport-Security" = "'max-age=31536000; includeSubDomains; preload'"
  }
  response_templates = {
    "application/json" = "{\"Location\":\"https://${var.dns_domain_name}/login?next=/credential-factory\"}" # a last resort
    #"application/json" = "{\"Location\":\"$context.authorizer.Location\"}"
  }
}

module "substrate-apigateway-authenticator" {
  apigateway_execution_arn = "${aws_api_gateway_deployment.intranet.execution_arn}/*"
  filename                 = "${path.module}/substrate-apigateway-authenticator.zip"
  name                     = "substrate-apigateway-authenticator"
  role_arn                 = data.aws_iam_role.substrate-apigateway-authenticator.arn
  source                   = "../../lambda-function/regional"
}

module "substrate-apigateway-authorizer" {
  #apigateway_execution_arn = "${aws_api_gateway_deployment.intranet.execution_arn}/*"
  #apigateway_execution_arn = "arn:aws:apigateway:${data.aws_region.current.name}::*"
  #apigateway_execution_arn = "arn:aws:apigateway:${data.aws_region.current.name}::/restapis/${aws_api_gateway_rest_api.intranet.id}/authorizers/${aws_api_gateway_authorizer.substrate.id}"
  apigateway_execution_arn = "arn:aws:execute-api:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:${aws_api_gateway_rest_api.intranet.id}/*"
  filename                 = "${path.module}/substrate-apigateway-authorizer.zip"
  name                     = "substrate-apigateway-authorizer"
  role_arn                 = data.aws_iam_role.substrate-apigateway-authorizer.arn
  source                   = "../../lambda-function/regional"
}

module "substrate-credential-factory" {
  apigateway_execution_arn = "${aws_api_gateway_deployment.intranet.execution_arn}/*"
  filename                 = "${path.module}/substrate-credential-factory.zip"
  name                     = "substrate-credential-factory"
  role_arn                 = data.aws_iam_role.substrate-credential-factory.arn
  source                   = "../../lambda-function/regional"
}

module "substrate-instance-factory" {
  apigateway_execution_arn = "${aws_api_gateway_deployment.intranet.execution_arn}/*"
  filename                 = "${path.module}/substrate-instance-factory.zip"
  name                     = "substrate-instance-factory"
  role_arn                 = data.aws_iam_role.substrate-instance-factory.arn
  source                   = "../../lambda-function/regional"
}

resource "aws_acm_certificate" "intranet" {
  domain_name       = var.dns_domain_name
  validation_method = "DNS"
}

resource "aws_acm_certificate_validation" "intranet" {
  certificate_arn         = aws_acm_certificate.intranet.arn
  validation_record_fqdns = [var.validation_fqdn]
}

resource "aws_api_gateway_account" "current" {
  depends_on          = [aws_cloudwatch_log_group.apigateway-welcome]
  cloudwatch_role_arn = data.aws_iam_role.apigateway.arn
}

resource "aws_api_gateway_authorizer" "substrate" {
  authorizer_credentials           = data.aws_iam_role.apigateway.arn
  authorizer_result_ttl_in_seconds = 0 # disabled because we need the authorizer to calculate context.authorizer.Location
  authorizer_uri                   = module.substrate-apigateway-authorizer.invoke_arn
  identity_source                  = "method.request.header.Host" # force the authorizer to run every time because this header is present every time
  name                             = "Substrate"
  rest_api_id                      = aws_api_gateway_rest_api.intranet.id
  type                             = "REQUEST"
}

resource "aws_api_gateway_base_path_mapping" "intranet" {
  api_id      = aws_api_gateway_rest_api.intranet.id
  stage_name  = aws_api_gateway_deployment.intranet.stage_name
  domain_name = aws_api_gateway_domain_name.intranet.domain_name
}

resource "aws_api_gateway_deployment" "intranet" {
  lifecycle {
    create_before_destroy = true
  }
  rest_api_id = aws_api_gateway_rest_api.intranet.id
  stage_name  = var.stage_name
  triggers = {
    redeployment = sha1(join(",", list(
      jsonencode(aws_api_gateway_authorizer.substrate),
      jsonencode(aws_api_gateway_integration.GET-credential-factory),
      jsonencode(aws_api_gateway_integration.GET-instance-factory),
      jsonencode(aws_api_gateway_integration.GET-login),
      jsonencode(aws_api_gateway_integration.POST-instance-factory),
      jsonencode(aws_api_gateway_integration.POST-login),
      jsonencode(aws_api_gateway_method.GET-credential-factory),
      jsonencode(aws_api_gateway_method.GET-instance-factory),
      jsonencode(aws_api_gateway_method.GET-login),
      jsonencode(aws_api_gateway_method.POST-instance-factory),
      jsonencode(aws_api_gateway_method.POST-login),
      jsonencode(aws_api_gateway_resource.credential-factory),
      jsonencode(aws_api_gateway_resource.instance-factory),
      jsonencode(aws_api_gateway_resource.login),
      jsonencode(aws_api_gateway_gateway_response.ACCESS_DENIED),
      jsonencode(aws_api_gateway_gateway_response.UNAUTHORIZED),
      jsonencode(aws_cloudwatch_log_group.apigateway),
    )))
  }
  variables = {
    "OAuthOIDCClientID"              = var.oauth_oidc_client_id
    "OAuthOIDCClientSecretTimestamp" = var.oauth_oidc_client_secret_timestamp
    "OktaHostname"                   = var.okta_hostname
    "SelectedRegions"                = join(",", var.selected_regions)
  }
}

resource "aws_api_gateway_domain_name" "intranet" {
  domain_name = var.dns_domain_name
  endpoint_configuration {
    types = ["REGIONAL"]
  }
  regional_certificate_arn = aws_acm_certificate_validation.intranet.certificate_arn
  security_policy          = "TLS_1_2"
}

# TODO add this header to every response, which doesn't seem possible to do with a GatewayResponse.
# "gatewayresponse.header.Strict-Transport-Security" = "'max-age=31536000; includeSubDomains; preload'"

resource "aws_api_gateway_gateway_response" "ACCESS_DENIED" {
  response_parameters = local.response_parameters
  response_templates  = local.response_templates
  response_type       = "ACCESS_DENIED"
  rest_api_id         = aws_api_gateway_rest_api.intranet.id
  status_code         = "302"
}

resource "aws_api_gateway_gateway_response" "UNAUTHORIZED" {
  response_parameters = local.response_parameters
  response_templates  = local.response_templates
  response_type       = "UNAUTHORIZED"
  rest_api_id         = aws_api_gateway_rest_api.intranet.id
  status_code         = "302"
}

resource "aws_api_gateway_integration" "GET-credential-factory" {
  credentials             = data.aws_iam_role.apigateway.arn
  http_method             = aws_api_gateway_method.GET-credential-factory.http_method
  integration_http_method = "POST"
  passthrough_behavior    = "NEVER"
  resource_id             = aws_api_gateway_resource.credential-factory.id
  rest_api_id             = aws_api_gateway_rest_api.intranet.id
  type                    = "AWS_PROXY"
  uri                     = module.substrate-credential-factory.invoke_arn
}

resource "aws_api_gateway_integration" "GET-instance-factory" {
  credentials             = data.aws_iam_role.apigateway.arn
  http_method             = aws_api_gateway_method.GET-instance-factory.http_method
  integration_http_method = "POST"
  passthrough_behavior    = "NEVER"
  resource_id             = aws_api_gateway_resource.instance-factory.id
  rest_api_id             = aws_api_gateway_rest_api.intranet.id
  type                    = "AWS_PROXY"
  uri                     = module.substrate-instance-factory.invoke_arn
}

resource "aws_api_gateway_integration" "GET-login" {
  credentials             = data.aws_iam_role.apigateway.arn
  http_method             = aws_api_gateway_method.GET-login.http_method
  integration_http_method = "POST"
  passthrough_behavior    = "NEVER"
  resource_id             = aws_api_gateway_resource.login.id
  rest_api_id             = aws_api_gateway_rest_api.intranet.id
  type                    = "AWS_PROXY"
  uri                     = module.substrate-apigateway-authenticator.invoke_arn
}

resource "aws_api_gateway_integration" "POST-instance-factory" {
  credentials             = data.aws_iam_role.apigateway.arn
  http_method             = aws_api_gateway_method.POST-instance-factory.http_method
  integration_http_method = "POST"
  passthrough_behavior    = "NEVER"
  resource_id             = aws_api_gateway_resource.instance-factory.id
  rest_api_id             = aws_api_gateway_rest_api.intranet.id
  type                    = "AWS_PROXY"
  uri                     = module.substrate-instance-factory.invoke_arn
}

resource "aws_api_gateway_integration" "POST-login" {
  credentials             = data.aws_iam_role.apigateway.arn
  http_method             = aws_api_gateway_method.POST-login.http_method
  integration_http_method = "POST"
  passthrough_behavior    = "NEVER"
  resource_id             = aws_api_gateway_resource.login.id
  rest_api_id             = aws_api_gateway_rest_api.intranet.id
  type                    = "AWS_PROXY"
  uri                     = module.substrate-apigateway-authenticator.invoke_arn
}

resource "aws_api_gateway_method" "GET-credential-factory" {
  authorization = "CUSTOM"
  authorizer_id = aws_api_gateway_authorizer.substrate.id
  http_method   = "GET"
  resource_id   = aws_api_gateway_resource.credential-factory.id
  rest_api_id   = aws_api_gateway_rest_api.intranet.id
}

resource "aws_api_gateway_method" "GET-instance-factory" {
  authorization = "CUSTOM"
  authorizer_id = aws_api_gateway_authorizer.substrate.id
  http_method   = "GET"
  resource_id   = aws_api_gateway_resource.instance-factory.id
  rest_api_id   = aws_api_gateway_rest_api.intranet.id
}

resource "aws_api_gateway_method" "GET-login" {
  authorization = "NONE"
  http_method   = "GET"
  resource_id   = aws_api_gateway_resource.login.id
  rest_api_id   = aws_api_gateway_rest_api.intranet.id
}

resource "aws_api_gateway_method" "POST-instance-factory" {
  authorization = "CUSTOM"
  authorizer_id = aws_api_gateway_authorizer.substrate.id
  http_method   = "POST"
  resource_id   = aws_api_gateway_resource.instance-factory.id
  rest_api_id   = aws_api_gateway_rest_api.intranet.id
}

resource "aws_api_gateway_method" "POST-login" {
  authorization = "NONE"
  http_method   = "POST"
  resource_id   = aws_api_gateway_resource.login.id
  rest_api_id   = aws_api_gateway_rest_api.intranet.id
}

resource "aws_api_gateway_method_settings" "intranet" {
  depends_on  = [aws_api_gateway_account.current]
  method_path = "*/*"
  rest_api_id = aws_api_gateway_rest_api.intranet.id
  settings {
    logging_level   = "INFO"
    metrics_enabled = false
  }
  stage_name = aws_api_gateway_deployment.intranet.stage_name
}

resource "aws_api_gateway_resource" "credential-factory" {
  parent_id   = aws_api_gateway_rest_api.intranet.root_resource_id
  path_part   = "credential-factory"
  rest_api_id = aws_api_gateway_rest_api.intranet.id
}

resource "aws_api_gateway_resource" "instance-factory" {
  parent_id   = aws_api_gateway_rest_api.intranet.root_resource_id
  path_part   = "instance-factory"
  rest_api_id = aws_api_gateway_rest_api.intranet.id
}

resource "aws_api_gateway_resource" "login" {
  parent_id   = aws_api_gateway_rest_api.intranet.root_resource_id
  path_part   = "login"
  rest_api_id = aws_api_gateway_rest_api.intranet.id
}

resource "aws_api_gateway_rest_api" "intranet" {
  endpoint_configuration {
    types = ["REGIONAL"]
  }
  name = "Intranet"
  tags = {
    Manager = "Terraform"
  }
}

resource "aws_cloudwatch_log_group" "apigateway" {
  name              = "API-Gateway-Execution-Logs_${aws_api_gateway_rest_api.intranet.id}/${var.stage_name}"
  retention_in_days = 1
  tags = {
    Manager = "Terraform"
  }
}

resource "aws_cloudwatch_log_group" "apigateway-welcome" {
  name              = "/aws/apigateway/welcome"
  retention_in_days = 1
  tags = {
    Manager = "Terraform"
  }
}

resource "aws_route53_record" "intranet" {
  alias {
    evaluate_target_health = true
    name                   = aws_api_gateway_domain_name.intranet.regional_domain_name
    zone_id                = aws_api_gateway_domain_name.intranet.regional_zone_id
  }
  latency_routing_policy {
    region = data.aws_region.current.name
  }
  name           = aws_api_gateway_domain_name.intranet.domain_name
  set_identifier = data.aws_region.current.name
  type           = "A"
  zone_id        = data.aws_route53_zone.intranet.id
}
`,
	}
}
