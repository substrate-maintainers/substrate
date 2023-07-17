package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/src-bin/substrate/accounts"
	"github.com/src-bin/substrate/authorizerutil"
	"github.com/src-bin/substrate/awscfg"
	"github.com/src-bin/substrate/awsorgs"
	"github.com/src-bin/substrate/federation"
	"github.com/src-bin/substrate/lambdautil"
	"github.com/src-bin/substrate/oauthoidc"
	"github.com/src-bin/substrate/roles"
)

//go:generate go run ../../tools/template/main.go -name accountsTemplate -package main accounts.html
//go:generate go run ../../tools/template/main.go -name accountsJavaScript -package main accounts.js

func accountsHandler(
	ctx context.Context,
	cfg *awscfg.Config,
	oc *oauthoidc.Client,
	event *events.APIGatewayProxyRequest,
) (*events.APIGatewayProxyResponse, error) {
	var err error

	accountId := event.QueryStringParameters["number"]
	roleName := event.QueryStringParameters["role"]
	if accountId != "" && roleName != "" {

		/*
			var cfg12h *awscfg.Config
			if cfg12h, err = awsiam.AllDayConfig(ctx, cfg); err != nil {
				return lambdautil.ErrorResponse(err)
			}
		*/

		// XXX uh oh, we can't naively do this because first we have to assume our role here and then we have to assume the requested role

		// We have to start from the user's configured starting point so that
		// all questions of authorization are deferred to AWS.
		if cfg, err = cfg.AssumeRole(
			ctx,
			event.RequestContext.AccountID,
			event.RequestContext.Authorizer[authorizerutil.RoleName].(string),
			time.Hour,
		); err != nil {
			return lambdautil.ErrorResponse(err)
		}

		roleArn := roles.ARN(accountId, roleName)
		cfg.Telemetry().SetFinalAccountId(accountId)
		cfg.Telemetry().SetFinalRoleName(roleArn)
		if cfg, err = cfg.AssumeRole(
			ctx,
			accountId,
			roleName,
			time.Hour,
		); err != nil {
			return lambdautil.ErrorResponse(err)
		}
		creds, err := cfg.Retrieve(ctx)
		if err != nil {
			return lambdautil.ErrorResponse(err)
		}

		var destination string // empty will land on the AWS Console homepage
		if next := event.QueryStringParameters["next"]; next != "" {
			if u, err := url.Parse(next); err == nil {
				if strings.HasSuffix(u.Host, "console.aws.amazon.com") { // don't be an open redirect
					destination = next
				}
			}
		}

		consoleSigninURL, err := federation.ConsoleSigninURL(
			creds,
			destination,
			event,
		)
		if err != nil {
			return lambdautil.ErrorResponse(err)
		}

		return &events.APIGatewayProxyResponse{
			Body: fmt.Sprintf("redirecting to %s", consoleSigninURL),
			Headers: map[string]string{
				"Content-Type":                   "text/plain",
				"Location":                       consoleSigninURL,
				"X-Substrate-Credentials-Expire": creds.Expires.Format(time.RFC3339),
			},
			StatusCode: http.StatusFound,
		}, nil
	}

	if cfg, err = cfg.OrganizationReader(ctx); err != nil {
		return lambdautil.ErrorResponse(err)
	}
	adminAccounts, serviceAccounts, substrateAccount, auditAccount, deployAccount, managementAccount, networkAccount, err := accounts.Grouped(ctx, cfg)
	if err != nil {
		return lambdautil.ErrorResponse(err)
	}

	body, err := lambdautil.RenderHTML(accountsTemplate(), struct {
		AdminAccounts, ServiceAccounts                                 []*awsorgs.Account
		SubstrateAccount                                               *awsorgs.Account
		AuditAccount, DeployAccount, ManagementAccount, NetworkAccount *awsorgs.Account
		RoleName                                                       string
	}{
		adminAccounts, serviceAccounts,
		substrateAccount,
		auditAccount, deployAccount, managementAccount, networkAccount,
		event.RequestContext.Authorizer[authorizerutil.RoleName].(string),
	})
	if err != nil {
		return nil, err
	}
	return &events.APIGatewayProxyResponse{
		Body:       body,
		Headers:    map[string]string{"Content-Type": "text/html"},
		StatusCode: http.StatusOK,
	}, nil

}

func init() {
	handlers["/accounts"] = accountsHandler
	handlers["/js/accounts.js"] = lambdautil.StaticHandler("application/javascript", accountsJavaScript())
}
