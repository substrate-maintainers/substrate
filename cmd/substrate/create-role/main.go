package createrole

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/src-bin/substrate/accounts"
	"github.com/src-bin/substrate/admin"
	"github.com/src-bin/substrate/awscfg"
	"github.com/src-bin/substrate/awsiam"
	"github.com/src-bin/substrate/awsorgs"
	"github.com/src-bin/substrate/cmdutil"
	"github.com/src-bin/substrate/jsonutil"
	"github.com/src-bin/substrate/policies"
	"github.com/src-bin/substrate/roles"
	"github.com/src-bin/substrate/tagging"
	"github.com/src-bin/substrate/ui"
	"github.com/src-bin/substrate/users"
	"github.com/src-bin/substrate/version"
)

func Main(ctx context.Context, cfg *awscfg.Config) {
	roleName := flag.String("role", "", "name of the IAM role to create")
	selector := accounts.NewSelector(accounts.SelectorUsage{
		AllDomains:      "create the role in all domains (potentially constrained by -environment and/or -quality)",
		Domains:         "only create this role in AWS accounts in this domain (may be repeated)",
		AllEnvironments: "create the role in all environments (potentially constrained by -domain and/or -quality)",
		Environments:    "only create this role in AWS accounts in this environment (may be repeated)",
		AllQualities:    "create the role in all qualities (potentially constrained by -domain and/or -environment)",
		Qualities:       "only create this role in AWS accounts of this quality (may be repeated)",
		Admin:           "create this role in the organization's admin account(s) (potentially constrained by -quality)",
		Management:      "create this role in the organization's management AWS account",
		Specials:        `create this role in a special AWS account (may be repeated; "deploy" and/or "network")`,
		Numbers:         "create this role in a specific AWS account, by 12-digit account number (may be repeated)",
	})
	humans := flag.Bool("humans", false, "allow humans with this role set in your IdP to assume this role via the Credential Factory (implies -admin)")
	awsServices := cmdutil.StringSlice("aws-service", `allow an AWS service (by URL; e.g. "ec2.amazonaws.com") to assume role (may be repeated)`)
	githubActions := cmdutil.StringSlice("github-actions", `allow GitHub Actions to assume this role in the context of the given GitHub organization and repository (separated by a literal '/'; may be repeated)`)
	assumeRolePolicyFilename := flag.String("assume-role-policy", "", "filename containing an assume-role policy to be merged into this role's final assume-role policy")
	quiet := flag.Bool("quiet", false, "suppress status and diagnostic output")
	flag.Usage = func() {
		ui.Print("Usage: substrate create-role [account selection flags] -role <role> [assume-role policy flags] [policy flags] [-quiet]")
		ui.Print("       [account selection flags]:  [-all-domains|-domain <domain> [...]]")
		ui.Print("                                   [-all-environments|-environment <environment> [...]]")
		ui.Print("                                   [-all-qualities|-quality <quality> [...]]")
		ui.Print("                                   [-admin [-quality <quality>]]")
		ui.Print("                                   [-management] [-special <special> [...]]")
		ui.Print("                                   [-number <number> [...]]")
		ui.Print("       [assume-role policy flags]: [-humans] [-aws-service <aws-service-url>] [-github-actions <org/repo>] [-assume-role-policy <filename>]")
		flag.PrintDefaults()
	}
	flag.Parse()
	version.Flag()
	if *quiet {
		ui.Quiet()
	}
	if *roleName == "" {
		ui.Fatal(`-role "..." is required`)
	}
	subs := make([]string, len(*githubActions))
	for i, repo := range *githubActions {
		if !strings.Contains(repo, "/") {
			ui.Fatal(`-github-actions "..." must contain a '/'`)
		}
		subs[i] = fmt.Sprintf("repo:%s:*", repo)
	}

	adminAccounts, serviceAccounts, _, deployAccount, managementAccount, networkAccount, err := accounts.Grouped(ctx, cfg)
	ui.Must(err)
	canned, err := admin.CannedPrincipals(ctx, cfg, false)
	ui.Must(err)

	// Collect configs for all the accounts selected by the given options.
	var selection []selectedAccount
	match := func(account *awsorgs.Account) (selectors []string, ok bool) {
		ok = true
		if *selector.AllDomains {
			selectors = append(selectors, "all-domains")
		} else if len(*selector.Domains) > 0 {
			if contains(*selector.Domains, account.Tags[tagging.Domain]) {
				selectors = append(selectors, "domain")
			} else {
				ok = false
			}
		} else {
			ok = false
		}
		if *selector.AllEnvironments {
			selectors = append(selectors, "all-environments")
		} else if len(*selector.Environments) > 0 {
			if contains(*selector.Environments, account.Tags[tagging.Environment]) {
				selectors = append(selectors, "environment")
			} else {
				ok = false
			}
		} else {
			ok = false
		}
		if *selector.AllQualities {
			selectors = append(selectors, "all-qualities")
		} else if len(*selector.Qualities) > 0 {
			if contains(*selector.Qualities, account.Tags[tagging.Quality]) {
				selectors = append(selectors, "quality")
			} else {
				ok = false
			}
		} else {
			ok = false
		}
		return
	}
	for _, account := range serviceAccounts {
		if selectors, ok := match(account); ok {
			selection = append(selection, selectedAccount{
				account:   account,
				cfg:       awscfg.Must(cfg.AssumeRole(ctx, aws.ToString(account.Id), roles.Administrator, time.Hour)),
				selectors: selectors,
			})
		}
	}
	if *selector.Management {
		selection = append(selection, selectedAccount{
			account:   managementAccount,
			cfg:       awscfg.Must(cfg.AssumeManagementRole(ctx, roles.OrganizationAdministrator, time.Hour)),
			selectors: []string{"management"},
		})
	}
	for _, special := range *selector.Specials {
		switch special {
		case accounts.Deploy:
			selection = append(selection, selectedAccount{
				account:   deployAccount,
				cfg:       awscfg.Must(cfg.AssumeSpecialRole(ctx, accounts.Deploy, roles.DeployAdministrator, time.Hour)),
				selectors: []string{"special"},
			})
		case accounts.Network:
			selection = append(selection, selectedAccount{
				account:   networkAccount,
				cfg:       awscfg.Must(cfg.AssumeSpecialRole(ctx, accounts.Network, roles.NetworkAdministrator, time.Hour)),
				selectors: []string{"special"},
			})
		default:
			ui.Fatal("creating additional roles in the audit account is not supported")
		}
	}
	for _, number := range *selector.Numbers {
		selection = append(selection, selectedAccount{
			account:   awsorgs.StringableZeroAccount(number),
			cfg:       awscfg.Must(cfg.AssumeRole(ctx, number, roles.Administrator, time.Hour)),
			selectors: []string{"number"},
		})
	}

	// Every role we create needs these minimal privileges in order to use
	// the Substrate command-line tools.
	minimalPolicy := &policies.Document{
		Statement: []policies.Statement{{
			Action: []string{
				"organizations:DescribeOrganization",
				"sts:AssumeRole",
			},
			Resource: []string{"*"},
		}},
	}

	// If this role's for humans to use via the IdP, create a role by the same
	// name in admin accounts that can't (necessarily) do the same things; its
	// purpose is to be allowed (later) to assume the roles created in the
	// various other accounts.
	adminPrincipals := &policies.Principal{AWS: []string{}}
	if *humans {
		ui.Spinf("finding or creating the %s role in your admin account(s) for humans to assume via your IdP", *roleName)
		for _, account := range adminAccounts {
			role, err := awsiam.EnsureRoleWithPolicy(
				ctx,
				awscfg.Must(cfg.AssumeRole(
					ctx,
					aws.ToString(account.Id),
					roles.Administrator,
					time.Hour,
				)),
				*roleName,
				policies.Merge(
					policies.AssumeRolePolicyDocument(canned.AdminRolePrincipals), // Administrator can do anything, after all
					policies.AssumeRolePolicyDocument(&policies.Principal{
						AWS: []string{
							roles.ARN(aws.ToString(account.Id), roles.Intranet),
							users.ARN(aws.ToString(account.Id), users.CredentialFactory),
						},
						Service: []string{"ec2.amazonaws.com"},
					}),
				),
				minimalPolicy,
			)
			ui.Must(err)
			//log.Printf("%+v", role)
			ui.Must(awsiam.TagRole(ctx, cfg, role.Name, tagging.Map{
				tagging.SubstrateAccountSelectors: "humans",
			}))
			_, err = awsiam.EnsureInstanceProfile(ctx, cfg, role.Name)
			ui.Must(err)
			adminPrincipals.AWS = append(adminPrincipals.AWS, role.Arn)
		}
		ui.Stop("ok")
	}

	// Create the role in each account, constructing the assume-role policy
	// uniquely for each one because certain aspects, e.g. the GitHub Actions
	// OAuth OIDC provider, may differ in the details from one account to
	// another.
	for _, selected := range selection {
		account := selected.account
		ui.Printf("constructing an assume-role policy for the %s role in %s", *roleName, account)

		// Construct the assume-role policy for this role as it will be created in
		// this account. This governs who may assume this role.
		assumeRolePolicy := policies.AssumeRolePolicyDocument(canned.AdminRolePrincipals) // Administrator can do anything, after all
		if *humans {
			ui.Printf("allowing humans to assume the %s role in %s via admin accounts and your IdP", *roleName, account)
			assumeRolePolicy = policies.Merge(
				assumeRolePolicy,
				policies.AssumeRolePolicyDocument(adminPrincipals),
			)
		}
		if len(*awsServices) > 0 {
			ui.Printf("allowing %s to assume the %s role in %s", strings.Join(*awsServices, ", "), *roleName, account)
			assumeRolePolicy = policies.Merge(
				assumeRolePolicy,
				policies.AssumeRolePolicyDocument(&policies.Principal{
					Service: jsonutil.StringSlice(*awsServices),
				}),
			)
		}
		if len(*githubActions) > 0 {
			ui.Printf(
				"allowing GitHub Actions to assume the %s role in %s on behalf of %s",
				*roleName,
				account,
				strings.Join(*githubActions, ", "),
			)
			arn, err := awsiam.EnsureOpenIDConnectProvider(
				ctx,
				selected.cfg,
				[]string{"sts.amazonaws.com"},
				[]string{"6938fd4d98bab03faadb97b34396831e3780aea1"},
				"https://token.actions.githubusercontent.com",
			)
			ui.Must(err)
			assumeRolePolicy = policies.Merge(
				assumeRolePolicy,
				&policies.Document{
					Statement: []policies.Statement{{
						Action: []string{"sts:AssumeRoleWithWebIdentity"},
						Condition: policies.Condition{"StringEquals": {
							"token.actions.githubusercontent.com:sub": subs,
						}},
						Principal: &policies.Principal{
							Federated: []string{arn},
						},
					}},
				},
			)
		}
		var filePolicy policies.Document
		if err := jsonutil.Read(*assumeRolePolicyFilename, &filePolicy); err == nil {
			ui.Printf("reading additional assume-role policy statements from %s", *assumeRolePolicyFilename)
			assumeRolePolicy = policies.Merge(assumeRolePolicy, &filePolicy)
		} else if !errors.Is(err, fs.ErrNotExist) {
			ui.Fatal(err)
		}
		//log.Print(jsonutil.MustString(assumeRolePolicy)) // TODO make the awsiam.EnsureRole function(s) sensitive to SUBSTRATE_DEBUG_ASSUME_ROLE_POLICY

		ui.Spinf("finding or creating the %s role in %s", *roleName, account)
		role, err := awsiam.EnsureRoleWithPolicy(
			ctx,
			selected.cfg,
			*roleName,
			assumeRolePolicy,
			minimalPolicy,
		)
		ui.Must(err)
		//log.Printf("%+v", role)
		ui.Must(awsiam.TagRole(ctx, selected.cfg, role.Name, tagging.Map{
			tagging.SubstrateAccountSelectors: strings.Join(selected.selectors, " "),
		}))
		for _, awsService := range *awsServices {
			if awsService == "ec2.amazonaws.com" {
				_, err = awsiam.EnsureInstanceProfile(ctx, selected.cfg, *roleName)
				ui.Must(err)
			}
		}
		ui.Stopf("ok")

		// TODO -administrator and -auditor canned attached policies, too, plus -policy to attach a policy everywhere (except, possibly, admin accounts)
	}

}

func contains(ss []string, s string) bool {
	for i := 0; i < len(ss); i++ {
		if ss[i] == s {
			return true
		}
	}
	return false
}

type selectedAccount struct {
	account   *awsorgs.Account
	cfg       *awscfg.Config
	selectors []string
}
