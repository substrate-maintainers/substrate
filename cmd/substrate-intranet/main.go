package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/src-bin/substrate/authorizerutil"
	"github.com/src-bin/substrate/awscfg"
	"github.com/src-bin/substrate/contextutil"
	"github.com/src-bin/substrate/oauthoidc"
	"github.com/src-bin/substrate/ui"
)

//go:generate go run ../../tools/dispatch-map/main.go .
//go:generate go run ../../tools/dispatch-map/main.go -function JavaScript -o dispatch-map-js.go .
//go:generate go run ../../tools/dispatch-map/main.go -function Main2 -o dispatch-map-main.go .

type Handler func(
	context.Context,
	*awscfg.Config,
	*oauthoidc.Client,
	*events.APIGatewayProxyRequest,
) (*events.APIGatewayProxyResponse, error)

var handlers = map[string]Handler{}

func main() {
	const (
		IntranetFunctionName                     = "Intranet"
		IntranetAPIGatewayAuthorizerFunctionName = "IntranetAPIGatewayAuthorizer"
		IntranetProxyFunctionNamePrefix          = "IntranetProxy-"
		SubstrateFunctionName                    = "Substrate"
		varName                                  = "AWS_LAMBDA_FUNCTION_NAME"
	)
	functionName := os.Getenv(varName)

	ctx := contextutil.WithValues(context.Background(), "substrate-intranet", "", "")

	cfg, err := awscfg.NewConfig(ctx)
	if err != nil {
		ui.Fatal(err)
	}

	clientId := os.Getenv(oauthoidc.OAuthOIDCClientId)
	var pathQualifier oauthoidc.PathQualifier
	switch oauthoidc.IdPName(clientId) {
	case oauthoidc.AzureAD:
		pathQualifier = oauthoidc.AzureADPathQualifier(os.Getenv(oauthoidc.AzureADTenantId))
	case oauthoidc.Google:
		pathQualifier = oauthoidc.GooglePathQualifier()
	case oauthoidc.Okta:
		pathQualifier = oauthoidc.OktaPathQualifier(os.Getenv(oauthoidc.OktaHostname))
	}
	oc, err := oauthoidc.NewClient(
		ctx,
		cfg,
		clientId,
		os.Getenv(oauthoidc.OAuthOIDCClientSecretTimestamp),
		pathQualifier,
	)
	if err != nil {
		ui.Fatal(err)
	}

	if functionName == IntranetAPIGatewayAuthorizerFunctionName {
		lambda.Start(authorizer(cfg, oc))

	} else if functionName == IntranetFunctionName {
		lambda.Start(func(ctx context.Context, event *events.APIGatewayProxyRequest) (*events.APIGatewayProxyResponse, error) {
			ctx = contextutil.WithValues(
				ctx,
				"substrate-intranet",
				event.Path,
				fmt.Sprint(event.RequestContext.Authorizer[authorizerutil.PrincipalId]),
			)

			// Send telemetry to Source & Binary (if enabled) for every
			// endpoint except /audit since part of its function is to
			// forward events to Source & Binary (if enabled).
			if event.Path != "/audit" {
				defer func() { go cfg.Telemetry().Post(ctx) }()
			}

			// New-style dispatch to handlers in their own packages.
			k := strings.SplitN(event.Path, "/", 3)[1]
			if k == "" {
				k = "index"
			}
			if f, ok := dispatchMapMain[k]; ok {
				return f(ctx, cfg, oc.Copy(), event)
			}

			// Old-style dispatch to handlers still in this package.
			if h, ok := handlers[event.Path]; ok {
				return h(ctx, cfg, oc.Copy(), event)
			}

			return &events.APIGatewayProxyResponse{
				Body:       "404 Not Found\n",
				Headers:    map[string]string{"Content-Type": "text/plain"},
				StatusCode: http.StatusNotFound,
			}, nil
		})

	} else if strings.HasPrefix(functionName, IntranetProxyFunctionNamePrefix) {
		//pathPart := strings.TrimPrefix(functionName, IntranetProxyFunctionNamePrefix)
		lambda.Start(proxy)

	} else if functionName == SubstrateFunctionName {
		lambda.Start(&Mux{
			Authorizer: authorizer2(cfg, oc),
			Handler: func(ctx context.Context, event *events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
				var principalId string
				if event.RequestContext.Authorizer != nil {
					principalId = fmt.Sprint(event.RequestContext.Authorizer.Lambda[authorizerutil.PrincipalId])
				}
				ctx = contextutil.WithValues(ctx, "substrate-intranet", event.RawPath, principalId)

				if path.Dir(event.RawPath) == "/js" && path.Ext(event.RawPath) == ".js" {
					k := strings.TrimSuffix(path.Base(event.RawPath), ".js")
					if f, ok := dispatchMapJavaScript[k]; ok {
						return f(ctx, cfg, oc.Copy(), event)
					}
				} else {
					k := strings.SplitN(event.RawPath, "/", 3)[1] // safe because there's always at least the leading '/'
					if k == "" {
						k = "index"
					}
					if f, ok := dispatchMapMain2[k]; ok {
						return f(ctx, cfg, oc.Copy(), event)
					}
				}

				return &events.APIGatewayV2HTTPResponse{
					Body:       fmt.Sprintf("%s not found\n", event.RawPath),
					Headers:    map[string]string{"Content-Type": "text/plain"},
					StatusCode: http.StatusNotFound,
				}, nil
			},
		})

	} else {
		lambda.Start(func(ctx context.Context, event *events.APIGatewayProxyRequest) (*events.APIGatewayProxyResponse, error) {
			return &events.APIGatewayProxyResponse{
				Body:       fmt.Sprintf("500 Internal Server Error\n\n%s=\"%s\" is not an expected configuration\n", varName, functionName),
				Headers:    map[string]string{"Content-Type": "text/plain"},
				StatusCode: http.StatusInternalServerError,
			}, nil
		})

	}
}
