package terraform

// managed by go generate; do not edit by hand

func lambdaFunctionRegionalTemplate() map[string]string {
	return map[string]string{
		"cloudwatch.tf": `
`,
		"main.tf":       `data "archive_file" "zip" {
  output_path = var.filename
  source_file = "${data.external.dirname.result.dirname}/${var.name}"
  type        = "zip"
}

data "external" "dirname" {
  program = ["/bin/sh", "-c", "echo \"{\\\"dirname\\\":\\\"$(which substrate-create-admin-account | xargs dirname)\\\"}\""]
}

resource "aws_cloudwatch_log_group" "lambda" {
  name              = "/aws/lambda/${var.name}"
  retention_in_days = 1
  tags = {
    Manager = "Terraform"
  }
}

resource "aws_lambda_function" "function" {
  depends_on       = [aws_cloudwatch_log_group.lambda]
  filename         = var.filename
  function_name    = var.name
  handler          = var.name
  memory_size      = 128 # default
  role             = var.role_arn
  runtime          = "go1.x"
  source_code_hash = filebase64sha256(var.filename)
  tags = {
    Manager = "Terraform"
    Name    = var.name
  }
  timeout = 60
}

resource "aws_lambda_permission" "permission" {
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.function.function_name
  principal     = "apigateway.amazonaws.com"
  #source_arn    = var.apigateway_execution_arn
}
`,
		"outputs.tf":    `output "function_arn" {
  value = aws_lambda_function.function.arn
}

output "invoke_arn" {
  value = aws_lambda_function.function.invoke_arn
}
`,
		"variables.tf":  `variable "apigateway_execution_arn" {
  type = string
}

variable "filename" {
  type = string
}

variable "name" {
  type = string
}

variable "role_arn" {
  type = string
}
`,
	}
}
