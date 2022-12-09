package deletestaticaccesskeys

import (
	"context"
	"flag"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/src-bin/substrate/awscfg"
	"github.com/src-bin/substrate/awsiam"
	"github.com/src-bin/substrate/fileutil"
	"github.com/src-bin/substrate/naming"
	"github.com/src-bin/substrate/roles"
	"github.com/src-bin/substrate/ui"
	"github.com/src-bin/substrate/users"
	"github.com/src-bin/substrate/version"
	"github.com/src-bin/substrate/versionutil"
)

func Main(ctx context.Context, cfg *awscfg.Config) {
	flag.Usage = func() {
		ui.Print("Usage: substrate delete-static-access-keys")
		flag.PrintDefaults()
	}
	flag.Parse()
	version.Flag()

	cfg = awscfg.Must(cfg.AssumeManagementRole(
		ctx,
		roles.OrganizationAdministrator,
		time.Hour,
	))
	versionutil.PreventDowngrade(ctx, cfg)

	cfg.Telemetry().FinalAccountId = aws.ToString(cfg.MustGetCallerIdentity(ctx).Account)
	cfg.Telemetry().FinalRoleName = roles.OrganizationAdministrator

	ui.Spin("deleting all access keys for the OrganizationAdministrator user")
	if err := awsiam.DeleteAllAccessKeys(ctx, cfg, users.OrganizationAdministrator); err != nil {
		log.Fatal(err)
	}
	ui.Stop("done")

	_, err := ui.Prompt("visit <https://console.aws.amazon.com/iam/home#/security_credentials> and delete all root access keys (which can't be deleted via the API) and press <enter>")
	if err != nil {
		log.Fatal(err)
	}

	intranetDNSDomainName, err := fileutil.ReadFile(naming.IntranetDNSDomainNameFilename)
	if err != nil {
		log.Fatal(err)
	}
	ui.Printf(
		"from now on, use `eval $(substrate credentials)` or <https://%s/credential-factory> to mint short-lived AWS access keys",
		strings.Trim(string(intranetDNSDomainName), "\r\n"),
	)

}
