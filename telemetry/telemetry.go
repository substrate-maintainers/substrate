package telemetry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/src-bin/substrate/contextutil"
	"github.com/src-bin/substrate/features"
	"github.com/src-bin/substrate/fileutil"
	"github.com/src-bin/substrate/naming"
	"github.com/src-bin/substrate/roles"
	"github.com/src-bin/substrate/version"
)

const Filename = "substrate.telemetry"

// Endpoint is an HTTP(S) URL where telemetry is sent, if it's not an empty
// string. The following values are useful:
// - "": Do not send telemetry.
// - "https://src-bin.org/telemetry/": Send telemetry to staging.
// - "https://src-bin.com/telemetry/": Send telemetry to production.
//
// The actual value is set at build time.
var Endpoint = ""

type Event struct {
	Command, Subcommand              string // e.g. "substrate" and "assume-role" or "substrate-intranet" and "InstanceFactory"
	Version                          string
	InitialAccountId, FinalAccountId string // avoid disclosing domain, environment, and quality
	EmailDomainName, EmailSHA256     string // avoid PII in local portion
	Prefix                           string
	InitialRoleName, FinalRoleName   string // "Administrator", "Auditor", or "Other" (avoid disclosing custom role names)
	IsEC2                            bool
	OS                               string        // runtime.GOOS
	Format                           string        `json:",omitempty"` // -format, if applicable
	mu                               sync.Mutex    `json:"-"`
	once                             sync.Once     `json:"-"`
	post                             int32         `json:"-"` // for compare-and-swap
	wait                             chan struct{} `json:"-"`
}

func NewEmptyEvent() *Event {
	return &Event{wait: make(chan struct{})}
}

func NewEvent(ctx context.Context) (*Event, error) {
	prefix, _ := naming.PrefixNoninteractive()
	e := &Event{
		Command:    contextutil.ValueString(ctx, contextutil.Command),
		Subcommand: contextutil.ValueString(ctx, contextutil.Subcommand),
		Version:    version.Version,
		Prefix:     prefix,
		OS:         runtime.GOOS,
		wait:       make(chan struct{}),
	}

	ctx, _ = context.WithTimeout(ctx, 100*time.Millisecond)
	for _, url := range []string{
		"http://169.254.169.254/latest/api/token",
		"http://[fd00:ec2::254]/latest/api/token",
	} {
		req, err := http.NewRequestWithContext(ctx, "PUT", url, nil)
		if err != nil {
			return nil, err
		}
		if _, err := http.DefaultClient.Do(req); err == nil {
			e.IsEC2 = true
			break
		}
	}

	return e, nil
}

func (e *Event) Post(ctx context.Context) error {
	if e == nil || endpoint(ctx) == "" {
		return nil
	}

	// Return early if we've already started to Post elsewhere.
	if !atomic.CompareAndSwapInt32(&e.post, 0, 1) {
		return nil
	}

	// Return early if we've already Wait-ed elsewhere.
	select {
	case <-e.wait:
		return nil
	default:
	}
	defer func() {
		//defer func() { recover() }() // allow closing e.wait multiple times
		e.once.Do(func() { close(e.wait) })
	}()

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.Command == "" {
		e.Command = contextutil.ValueString(ctx, contextutil.Command)
	}
	if e.Subcommand == "" {
		e.Subcommand = contextutil.ValueString(ctx, contextutil.Subcommand)
	}

	b := &bytes.Buffer{}
	if err := json.NewEncoder(b).Encode(e); err != nil {
		return err
	}
	ctx, _ = context.WithTimeout(ctx, time.Second)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint(ctx), b)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	_, err = http.DefaultClient.Do(req)
	return err
}

func (e *Event) PostWait(ctx context.Context) error {
	if err := e.Post(ctx); err != nil {
		return err
	}
	return e.Wait(ctx)
}

func (e *Event) SetInitialAccountId(accountId string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.InitialAccountId = accountId
	if e.FinalAccountId == "" {
		e.FinalAccountId = accountId
	}
}
func (e *Event) SetFinalAccountId(accountId string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.FinalAccountId = accountId
}

func (e *Event) SetEmailDomainName(email string) {
	if ss := strings.Split(email, "@"); len(ss) == 2 {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.EmailDomainName = ss[1]
	}
}

func (e *Event) SetEmailSHA256(email string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.EmailSHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte(email)))
}

func (e *Event) SetInitialRoleName(roleArn string) (err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.InitialRoleName, err = roleNameFromArn(roleArn)
	if e.FinalRoleName == "" {
		e.FinalRoleName, err = roleNameFromArn(roleArn)
	}
	return
}

func (e *Event) SetFinalRoleName(roleArn string) (err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.FinalRoleName, err = roleNameFromArn(roleArn)
	return
}

func (e *Event) Wait(ctx context.Context) error {
	if e == nil || endpoint(ctx) == "" {
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-e.wait:
	}
	return nil
}

func endpoint(ctx context.Context) string {
	if !features.Telemetry.Enabled() {
		return ""
	}
	if features.ProxyTelemetry.Enabled() && !contextutil.IsIntranet(ctx) { // feature enabled and outside the Intranet
		if host, err := naming.IntranetDNSDomainName(); err == nil { // and knowing the Intranet's hostname
			u := &url.URL{
				Scheme: "https",
				Host:   host,
				Path:   "/audit",
			}
			return u.String()
		}
	}
	return Endpoint // without the feature flag, in the Intranet, or before the Intranet exists, submit telemetry directly
}

func init() {
	fileutil.Remove(Filename)
}

func roleNameFromArn(roleArn string) (string, error) {
	parsed, err := arn.Parse(roleArn)
	if err != nil {
		return "", err
	}
	ss := strings.Split(parsed.Resource, "/")
	if len(ss) < 2 {
		return "", errors.New("arn: not enough sections") // <https://github.com/aws/aws-sdk-go-v2/blob/v1.15.0/aws/arn/arn.go#L23>
	}
	switch ss[0] {
	case "assumed-role", "role":
	default:
		return "", fmt.Errorf("%q is not an IAM role ARN", roleArn)
	}
	switch ss[1] {
	case roles.Administrator, roles.Auditor:
		return ss[1], nil
	}
	return "Other", nil // don't disclose customer-defined role names
}
