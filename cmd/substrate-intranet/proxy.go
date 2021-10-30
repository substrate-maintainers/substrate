package main

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/src-bin/substrate/lambdautil"
)

func proxy(ctx context.Context, event *events.APIGatewayProxyRequest) (*events.APIGatewayProxyResponse, error) {

	u, err := url.Parse(os.Getenv("PROXY_DESTINATION_URL"))
	if err != nil {
		return lambdautil.ErrorResponse(err)
	}
	uOriginal := u.String()
	_ = uOriginal
	if proxyPathPrefix := os.Getenv("PROXY_PATH_PREFIX"); os.Getenv("STRIP_PATH_PREFIX") == "true" {
		u.Path = path.Join(u.Path, strings.TrimPrefix(event.Path, proxyPathPrefix))
	} else {
		u.Path = path.Join(u.Path, event.Path)
	}
	// TODO figure out why trailing slashes are stripped and whether it matters.

	req, err := http.NewRequest(event.HTTPMethod, u.String(), strings.NewReader(event.Body))
	req.Header.Add("X-Substrate-Intranet-Proxy-Principal", event.RequestContext.Authorizer["principalId"].(string))
	for name, values := range event.MultiValueHeaders {
		if strings.ToLower(name) == "cookie" {
			continue
		}
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return lambdautil.ErrorResponse(err)
	}
	headers := map[string]string{}
	for name, values := range resp.Header {
		if len(values) > 0 { // headers must be unique according to the return type, which will be a problem for Set-Cookie headers eventually
			headers[name] = values[0]
		}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return lambdautil.ErrorResponse(err)
	}
	return &events.APIGatewayProxyResponse{
		Body:       string(body),
		Headers:    headers,
		StatusCode: resp.StatusCode,
	}, nil

}
