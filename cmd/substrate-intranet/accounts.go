package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go/service/organizations"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/src-bin/substrate/accounts"
	"github.com/src-bin/substrate/awsorgs"
	"github.com/src-bin/substrate/awssessions"
	"github.com/src-bin/substrate/awssts"
	"github.com/src-bin/substrate/lambdautil"
	"github.com/src-bin/substrate/roles"
)

//go:generate go run ../../tools/template/main.go -name accountsTemplate -package main accounts.html

func accountsHandler(ctx context.Context, event *events.APIGatewayProxyRequest) (*events.APIGatewayProxyResponse, error) {

	accountId := event.QueryStringParameters["number"]
	roleName := event.QueryStringParameters["role"]
	if accountId != "" && roleName != "" {
		sess, err := awssessions.NewSession(awssessions.Config{})
		if err != nil {
			return lambdautil.ErrorResponse(err) // TODO remove
			return nil, err
		}
		svc := sts.New(sess)

		assumedRole, err := awssts.AssumeRole(
			svc,
			roles.Arn(accountId, roleName),
			fmt.Sprint(event.RequestContext.Authorizer["principalId"]),
			3600, // AWS-enforced maximum when crossing accounts per <https://aws.amazon.com/premiumsupport/knowledge-center/iam-role-chaining-limit/> // TODO 43200?
		)
		if err != nil {
			return lambdautil.ErrorResponse(err)
		}
		credentials := assumedRole.Credentials

		consoleSigninURL, err := awssts.ConsoleSigninURL(svc, credentials, "")
		if err != nil {
			return lambdautil.ErrorResponse(err)
		}

		return &events.APIGatewayProxyResponse{
			Body: fmt.Sprintf("redirecting to %s", consoleSigninURL),
			Headers: map[string]string{
				"Content-Type": "text/plain",
				"Location":     consoleSigninURL,
			},
			StatusCode: http.StatusFound,
		}, nil
	}

	sess, err := awssessions.InManagementAccount(roles.OrganizationReader, awssessions.Config{})
	if err != nil {
		return nil, err
	}
	svc := organizations.New(sess)

	adminAccounts, serviceAccounts, auditAccount, deployAccount, managementAccount, networkAccount, err := accounts.Grouped(svc)
	if err != nil {
		return lambdautil.ErrorResponse(err)
	}

	body, err := lambdautil.RenderHTML(accountsTemplate(), struct {
		AdminAccounts, ServiceAccounts                                 []*awsorgs.Account
		AuditAccount, DeployAccount, ManagementAccount, NetworkAccount *awsorgs.Account
	}{
		adminAccounts, serviceAccounts,
		auditAccount, deployAccount, managementAccount, networkAccount,
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
}
