package main

import (
	"context"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/src-bin/substrate/oauthoidc"
)

func handle(ctx context.Context, event *events.APIGatewayCustomAuthorizerRequestTypeRequest) (*events.APIGatewayCustomAuthorizerResponse, error) {

	/*
		clientSecret, err := awssecretsmanager.GetSecretValue(fmt.Sprintf(
			"OktaClientSecret-%s",
			event.StageVariables["OktaClientID"],
		))
		if err != nil {
			return nil, err
		}
	*/
	clientSecret := "mFdL4HOHV5OquQVMm9SZd9r8RT9dLTccfTxPrfWc" // XXX
	c := oauthoidc.NewClient(
		event.StageVariables["OktaHostname"],
		oauthoidc.OktaPathQualifier("/oauth2/default"),
		event.StageVariables["OktaClientID"],
		clientSecret,
	)

	accessToken := &oauthoidc.OktaAccessToken{}
	idToken := &oauthoidc.OktaIDToken{}
	req := &http.Request{Header: http.Header{
		"Cookie": event.MultiValueHeaders["cookie"], // beware the case-sensitivity
	}}
	for _, cookie := range req.Cookies() {
		switch cookie.Name {
		case "a":
			if _, err := oauthoidc.ParseAndVerifyJWT(cookie.Value, c, accessToken); err != nil {
				return nil, err
			}
		case "id":
			if _, err := oauthoidc.ParseAndVerifyJWT(cookie.Value, c, idToken); err != nil {
				return nil, err
			}
		}
	}

	context := map[string]interface{}{}
	var err error
	if context["AccessToken"], err = accessToken.JSONString(); err != nil {
		return nil, err
	}
	if context["IDToken"], err = idToken.JSONString(); err != nil {
		return nil, err
	}
	return &events.APIGatewayCustomAuthorizerResponse{
		Context: context,
		PolicyDocument: events.APIGatewayCustomAuthorizerPolicy{
			Statement: []events.IAMPolicyStatement{
				{
					Action:   []string{"execute-api:Invoke"},
					Effect:   "Allow",
					Resource: []string{event.MethodArn},
				},
			},
			Version: "2012-10-17",
		},
		PrincipalID: accessToken.Subject,
	}, nil
}

func main() {
	lambda.Start(handle)
}
