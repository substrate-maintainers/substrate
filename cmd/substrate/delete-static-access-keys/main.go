package deletestaticaccesskeys

import (
	"context"
	"flag"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/src-bin/substrate/awscfg"
	"github.com/src-bin/substrate/awsiam"
	"github.com/src-bin/substrate/awssessions"
	"github.com/src-bin/substrate/cmdutil"
	"github.com/src-bin/substrate/fileutil"
	"github.com/src-bin/substrate/naming"
	"github.com/src-bin/substrate/roles"
	"github.com/src-bin/substrate/ui"
	"github.com/src-bin/substrate/users"
	"github.com/src-bin/substrate/version"
)

func Main(ctx context.Context, cfg *awscfg.Main) {
	cmdutil.MustChdir()
	flag.Parse()
	version.Flag()

	sess := awssessions.Must(awssessions.InManagementAccount(roles.OrganizationAdministrator, awssessions.Config{}))

	go cfg.Telemetry().Post(ctx) // post earlier, finish earlier

	ui.Spin("deleting all access keys for the OrganizationAdministrator user")
	if err := awsiam.DeleteAllAccessKeys(
		iam.New(sess),
		users.OrganizationAdministrator,
	); err != nil {
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
		"from now on, use substrate-credentials or <https://%s/credential-factory> to mint short-lived AWS access keys",
		strings.Trim(string(intranetDNSDomainName), "\r\n"),
	)

}
