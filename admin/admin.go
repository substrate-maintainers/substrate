package admin

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"sort"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/organizations"
	"github.com/src-bin/substrate/accounts"
	"github.com/src-bin/substrate/awsiam"
	"github.com/src-bin/substrate/awsorgs"
	"github.com/src-bin/substrate/awssessions"
	"github.com/src-bin/substrate/choices"
	"github.com/src-bin/substrate/jsonutil"
	"github.com/src-bin/substrate/policies"
	"github.com/src-bin/substrate/regions"
	"github.com/src-bin/substrate/roles"
	"github.com/src-bin/substrate/tags"
	"github.com/src-bin/substrate/terraform"
	"github.com/src-bin/substrate/ui"
	"github.com/src-bin/substrate/users"
)

func CannedAssumeRolePolicyDocuments(sess *session.Session, bootstrapping bool) (
	canned struct{ AdminRolePrincipals, AuditorRolePrincipals, OrgAccountPrincipals *policies.Document }, // TODO different names without "Principals"?
	err error,
) {
	cp, err := cannedPrincipals(organizations.New(sess), bootstrapping)

	var extraAdmin, extraAuditor policies.Document
	if err := jsonutil.Read("substrate.Administrator.assume-role-policy.json", &extraAdmin); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			ui.Printf("substrate.Administrator.assume-role-policy.json not found; create it if you wish to customize who can assume Administrator roles")
		} else {
			ui.Printf("error processing substrate.Administrator.assume-role-policy.json: %v", err)
		}
	}
	if err := jsonutil.Read("substrate.Auditor.assume-role-policy.json", &extraAuditor); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			ui.Printf("substrate.Auditor.assume-role-policy.json not found; create it if you wish to customize who can assume Auditor roles")
		} else {
			ui.Printf("error processing substrate.Auditor.assume-role-policy.json: %v", err)
		}
	}
	//log.Printf("%+v", extraAdmin)
	//log.Printf("%+v", extraAuditor)

	canned.AdminRolePrincipals = policies.Merge(
		policies.AssumeRolePolicyDocument(cp.AdminRolePrincipals),
		&extraAdmin,
	)
	canned.AuditorRolePrincipals = policies.Merge(
		policies.AssumeRolePolicyDocument(cp.AuditorRolePrincipals),
		&extraAuditor,
	)
	//log.Printf("%+v", canned.AdminRolePrincipals)
	//log.Printf("%+v", canned.AuditorRolePrincipals)

	canned.OrgAccountPrincipals = policies.AssumeRolePolicyDocument(cp.OrgAccountPrincipals)
	return canned, err
}

// EnsureAdminRolesAndPolicies creates or updates the entire matrix of
// Administrator roles and policies to allow the management account and admin
// accounts to move fairly freely throughout the organization.  The given
// session must be for the OrganizationAdministrator user or role in the management
// account. If doCloudWatch is true, it'll also reconfigure all the CloudWatch
// cross-account, cross-region roles; this is slow so it's only done when a new
// AWS account's being created since otherwise it's a no-op.
func EnsureAdminRolesAndPolicies(sess *session.Session, doCloudWatch bool) {
	svc := organizations.New(sess)

	// Gather lists of accounts.  These are used below in configuring policies
	// to allow cross-account access.  On the first run they're basically
	// no-ops but on subsequent runs this is key to not undoing the work of
	// substrate-create-account and substrate-create-admin-account.
	canned, err := CannedAssumeRolePolicyDocuments(
		sess,
		true, // can always be true because we never jump from Intranet to Administrator in other accounts
	)
	if err != nil {
		log.Fatal(err)
	}

	// Admin accounts, once they exist, are going to need to be able to assume
	// a role in the management account.  Because no admin accounts exist yet, this
	// is something of a no-op but it provides a place for them to attach
	// themselves as they're created.
	ui.Spin("finding or creating a role to allow admin accounts to administer your organization")
	role, err := awsiam.EnsureRoleWithPolicy(
		iam.New(sess),
		roles.OrganizationAdministrator,
		canned.AdminRolePrincipals,
		&policies.Document{
			Statement: []policies.Statement{{
				Action:   []string{"*"},
				Resource: []string{"*"},
			}},
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	ui.Stopf("role %s", role.Name)
	//log.Printf("%+v", role)

	// Ensure admin accounts can find other accounts in the organization without full administrative
	// privileges.  This should be directly possible via organizations:RegisterDelegatedAdministrator
	// but that API appears to just not work the way
	// <https://docs.aws.amazon.com/organizations/latest/userguide/orgs_integrated-services-list.html>
	// implies it does.  As above, this is almost a no-op for now because, while it grants access to
	// the special accounts, no admin accounts exist yet.
	ui.Spin("finding or creating a role to allow account discovery within your organization")
	role, err = awsiam.EnsureRoleWithPolicy(
		iam.New(sess),
		roles.OrganizationReader,
		canned.OrgAccountPrincipals,
		&policies.Document{
			Statement: []policies.Statement{{
				Action: []string{
					"organizations:DescribeAccount",
					"organizations:DescribeOrganization",
					"organizations:ListAccounts",
					"organizations:ListTagsForResource",
				},
				Resource: []string{"*"},
			}},
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	ui.Stopf("role %s", role.Name)
	//log.Printf("%+v", role)
	if doCloudWatch {
		ui.Spin("finding or creating a role to allow CloudWatch to discover accounts within your organization, too")
		role, err = awsiam.EnsureRoleWithPolicy(
			iam.New(sess),
			"CloudWatch-CrossAccountSharing-ListAccountsRole",
			canned.OrgAccountPrincipals,
			&policies.Document{
				Statement: []policies.Statement{{
					Action: []string{
						"organizations:ListAccounts",
						"organizations:ListAccountsForParent",
					},
					Resource: []string{"*"},
				}},
			},
		)
		if err != nil {
			log.Fatal(err)
		}
		ui.Stopf("role %s", role.Name)
		//log.Printf("%+v", role)
	} else {
		ui.Print("not managing CloudWatch cross-account sharing roles because no account was created")
	}

	// Ensure admin accounts can get into the audit account to look at
	// CloudTrail logs.  It's unfortunate that we can't grant admin accounts
	// direct access to the CloudTrail S3 bucket but, since the objects are
	// unavoidably owned by CloudTrail and merely grant access to the bucket
	// owner, bucket policy is powerless to delegate s3:GetObject to the rest
	// of the organization.  Per
	// - <https://aws.amazon.com/premiumsupport/knowledge-center/s3-cross-account-access-denied/>
	// - <https://aws.amazon.com/premiumsupport/knowledge-center/s3-bucket-owner-access/>
	// there is no affordance for making CloudTrail do what I want.  Oh well.
	// This is not simply EnsureAuditorRole because in the audit account we
	// actually do want auditors to be able to read from S3, etc.
	ui.Spin("finding or creating a role to allow admin accounts to audit your organization")
	auditAccount, err := awsorgs.FindSpecialAccount(svc, accounts.Audit)
	if err != nil {
		log.Fatal(err)
	}
	{
		bucketName := fmt.Sprintf("%s-cloudtrail", choices.Prefix()) // TODO don't repeat cmd/substrate/bootstrap-management-account/main.go
		svc := iam.New(awssessions.AssumeRole(
			sess,
			aws.StringValue(auditAccount.Id),
			roles.OrganizationAccountAccessRole,
		))
		role, err = awsiam.EnsureRoleWithPolicy(
			svc,
			roles.Auditor,
			canned.AuditorRolePrincipals,
			&policies.Document{
				Statement: []policies.Statement{
					{
						Action: []string{
							"s3:GetBucketLocation",
							"s3:GetObject",
							"s3:ListBucket",
							"s3:ListBucketMultipartUploads",
							"s3:ListMultipartUploadParts",
							"s3:AbortMultipartUpload",
							"s3:PutObject",
						},
						Resource: []string{"*"},
					},
					{
						Effect: policies.Deny,
						Action: []string{"s3:PutObject"},
						Resource: []string{
							fmt.Sprintf("arn:aws:s3:::%s/AWSLogs/*", bucketName),
						},
					},
				},
			},
		)
		if err != nil {
			log.Fatal(err)
		}
		if err := awsiam.AttachRolePolicy(svc, roles.Auditor, "arn:aws:iam::aws:policy/ReadOnlyAccess"); err != nil {
			log.Fatal(err)
		}
		/*
			if err := awsiam.AttachRolePolicy(svc, roles.Auditor, "arn:aws:iam::aws:policy/SecurityAudit"); err != nil {
				log.Fatal(err)
			}
		*/
	}
	ui.Stopf("role %s", role.Name)
	//log.Printf("%+v", role)

	// Ensure all admin accounts and the management account can administer the
	// deploy and network accounts.
	ui.Spin("finding or creating a role to allow admin accounts to administer your deploy artifacts")
	deployAccount, err := awsorgs.FindSpecialAccount(svc, accounts.Deploy)
	if err != nil {
		log.Fatal(err)
	}
	role, err = awsiam.EnsureRoleWithPolicy(
		iam.New(awssessions.AssumeRole(
			sess,
			aws.StringValue(deployAccount.Id),
			roles.OrganizationAccountAccessRole,
		)),
		roles.DeployAdministrator,
		canned.AdminRolePrincipals,
		&policies.Document{
			Statement: []policies.Statement{{
				Action:   []string{"*"},
				Resource: []string{"*"},
			}},
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	ui.Stopf("role %s", role.Name)
	//log.Printf("%+v", role)
	ui.Spin("finding or creating a role to allow admin accounts to audit your deploy artifacts")
	role, err = EnsureAuditorRole(
		iam.New(awssessions.AssumeRole(
			sess,
			aws.StringValue(deployAccount.Id),
			roles.OrganizationAccountAccessRole,
		)),
		canned.OrgAccountPrincipals,
	)
	if err != nil {
		log.Fatal(err)
	}
	ui.Stopf("role %s", role.Name)
	//log.Printf("%+v", role)
	ui.Spin("finding or creating a role to allow admin accounts to administer your networks")
	networkAccount, err := awsorgs.FindSpecialAccount(svc, accounts.Network)
	if err != nil {
		log.Fatal(err)
	}
	role, err = awsiam.EnsureRoleWithPolicy(
		iam.New(awssessions.AssumeRole(
			sess,
			aws.StringValue(networkAccount.Id),
			roles.OrganizationAccountAccessRole,
		)),
		roles.NetworkAdministrator,
		canned.AdminRolePrincipals,
		&policies.Document{
			Statement: []policies.Statement{{
				Action:   []string{"*"},
				Resource: []string{"*"},
			}},
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	ui.Stopf("role %s", role.Name)
	//log.Printf("%+v", role)
	ui.Spin("finding or creating a role to allow admin accounts to audit your networks (and Terraform code to discover them)")
	role, err = EnsureAuditorRole(
		iam.New(awssessions.AssumeRole(
			sess,
			aws.StringValue(networkAccount.Id),
			roles.OrganizationAccountAccessRole,
		)),
		canned.OrgAccountPrincipals,
	)
	if err != nil {
		log.Fatal(err)
	}
	ui.Stopf("role %s", role.Name)
	//log.Printf("%+v", role)

	allAccounts, err := awsorgs.ListAccounts(svc)
	if err != nil {
		log.Fatal(err)
	}

	// Loop through every non-admin, non-special account and authorize all the
	// Administrator roles to assume roles there.
	ui.Spinf("finding or creating Administrator and Auditor roles in all other accounts")
	terraformPrincipals := make([]string, 0, len(allAccounts))
	for _, account := range allAccounts {

		// Special accounts have special administrator role names but, still,
		// some of them should be allowed to run Terraform.
		if account.Tags[tags.SubstrateSpecialAccount] != "" {
			switch account.Tags[tags.SubstrateSpecialAccount] {
			case accounts.Deploy:
				terraformPrincipals = append(terraformPrincipals, roles.Arn(aws.StringValue(account.Id), roles.DeployAdministrator))
			case accounts.Management:
				terraformPrincipals = append(
					terraformPrincipals,
					roles.Arn(aws.StringValue(account.Id), roles.OrganizationAdministrator),
					users.Arn(aws.StringValue(account.Id), users.OrganizationAdministrator),
				)
			case accounts.Network:
				terraformPrincipals = append(terraformPrincipals, roles.Arn(aws.StringValue(account.Id), roles.NetworkAdministrator))
			}
			continue
		}

		// Every other account uses the role name "Administrator" and uses it
		// to run Terraform.
		terraformPrincipals = append(terraformPrincipals, roles.Arn(aws.StringValue(account.Id), roles.Administrator))

		// But the Administrator role in admin accounts is created during IdP
		// configuration so there's nothing more for us to do here.
		if account.Tags[tags.Domain] == "admin" {
			continue
		}

		// TODO if we try this block with Administrator as well as
		// OrganizationAccountAccessRole before giving up we can end up with
		// folks' original accounts in a much more managed (though still
		// untagged) state.
		svc := iam.New(awssessions.AssumeRole(
			sess,
			aws.StringValue(account.Id),
			roles.OrganizationAccountAccessRole,
		))
		if _, err := EnsureAdministratorRole(svc, canned.AdminRolePrincipals); err != nil {
			ui.Printf(
				"could not create the Administrator role in account %s; it might be because this account has only half-joined the organization",
				account.Id,
			)
		}
		if _, err := EnsureAuditorRole(svc, canned.AuditorRolePrincipals); err != nil {
			ui.Printf(
				"could not create the Auditor role in account %s; it might be because this account has only half-joined the organization",
				account.Id,
			)
		}
	}
	ui.Stop("ok")

	if doCloudWatch {

		ui.Spinf("finding or creating the CloudWatch-CrossAccountSharingRole role in all accounts")
		org, err := awsorgs.DescribeOrganization(svc)
		if err != nil {
			log.Fatal(err)
		}
		for _, account := range allAccounts {
			var svc *iam.IAM
			if aws.StringValue(account.Id) == aws.StringValue(org.MasterAccountId) {
				svc = iam.New(sess)
			} else {
				svc = iam.New(awssessions.AssumeRole(
					sess,
					aws.StringValue(account.Id),
					roles.OrganizationAccountAccessRole,
				))
			}
			if _, err := EnsureCloudWatchCrossAccountSharingRole(svc, canned.OrgAccountPrincipals); err != nil {
				ui.Printf(
					"could not create the CloudWatch-CrossAccountSharingRole role in account %s; it might be because this account has only half-joined the organization",
					account.Id,
				)
			}
		}
		ui.Stop("ok")

		// Create the service-linked role CloudWatch will actually use to read logs
		// and metrics from other accounts in your organization.
		ui.Spin("finding or creating CloudWatch's service-linked role for cross-account log and metric access")
		for _, account := range allAccounts {
			if account.Tags[tags.Domain] != "admin" {
				continue
			}
			svc := iam.New(awssessions.AssumeRole(
				sess,
				aws.StringValue(account.Id),
				roles.OrganizationAccountAccessRole,
			))
			if _, err := awsiam.EnsureServiceLinkedRole(
				svc,
				"AWSServiceRoleForCloudWatchCrossAccount",
				"cloudwatch-crossaccount.amazonaws.com",
			); err != nil {
				log.Fatal(err)
			}
		}
		ui.Stopf("ok")

	} else {
		ui.Print("not managing CloudWatch cross-account sharing roles because no account was created")
	}

	// Ensure every account can run Terraform with remote state centralized
	// in the deploy account.
	ui.Spin("finding or creating an IAM role for Terraform to use to manage remote state")
	sort.Strings(terraformPrincipals) // to avoid spurious policy diffs
	//log.Printf("%+v", terraformPrincipals)
	var resources []string
	for _, region := range regions.All() { // we can't use regions.Selected() in the first substrate-bootstrap-management-account
		bucketName := terraform.S3BucketName(region)
		resources = append(
			resources,
			fmt.Sprintf("arn:aws:s3:::%s", bucketName),
			fmt.Sprintf("arn:aws:s3:::%s/*", bucketName),
		)
	}
	role, err = awsiam.EnsureRoleWithPolicy(
		iam.New(awssessions.AssumeRole(
			sess,
			aws.StringValue(deployAccount.Id),
			roles.OrganizationAccountAccessRole,
		)),
		roles.TerraformStateManager,
		policies.AssumeRolePolicyDocument(&policies.Principal{AWS: terraformPrincipals}),
		&policies.Document{
			Statement: []policies.Statement{
				{
					Action: []string{"dynamodb:GetItem", "dynamodb:PutItem", "dynamodb:DeleteItem"},
					Resource: []string{
						fmt.Sprintf("arn:aws:dynamodb:*:*:table/%s", terraform.DynamoDBTableName),
					},
				},
				{
					Action:   []string{"s3:DeleteObject", "s3:GetObject", "s3:ListBucket", "s3:PutObject"},
					Resource: resources,
				},
			},
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	ui.Stopf("role %s", role.Name)
	//log.Printf("%+v", role)

}

// EnsureAdministratorRole creates the Administrator role in the account
// referenced by the given IAM client.  The only restrictions on the APIs
// this role may call are set by the organization's service control policies.
func EnsureAdministratorRole(svc *iam.IAM, assumeRolePolicyDocument *policies.Document) (*awsiam.Role, error) {
	return awsiam.EnsureRoleWithPolicy(
		svc,
		roles.Administrator,
		assumeRolePolicyDocument,
		&policies.Document{
			Statement: []policies.Statement{{
				Action:   []string{"*"},
				Resource: []string{"*"},
			}},
		},
	)
}

// EnsureAuditorRole creates the Auditor role in the account referenced by the
// given IAM client.  This role will be allowed to call all read-only APIs as
// defined by the AWS-managed ReadOnlyAccess policy except the list of
// sensitive read-only APIs identified by Alestic and captured in
// DenySensitiveReadsPolicyDocument. It will also be allowed to assume roles
// via AllowAssumeRolePolicyDocument but the roles that allows it to assume
// them will (presumably) also be read-only Auditor-like roles.
func EnsureAuditorRole(svc *iam.IAM, assumeRolePolicyDocument *policies.Document) (*awsiam.Role, error) {
	role, err := awsiam.EnsureRoleWithPolicy(
		svc,
		roles.Auditor,
		assumeRolePolicyDocument,
		policies.Merge(
			AllowAssumeRolePolicyDocument, // TODO set a permissions boundary to keep it read-only even if the role that allows Auditor to assume it is more permissive
			DenySensitiveReadsPolicyDocument,
		),
	)
	if err != nil {
		return nil, err
	}
	if err := awsiam.AttachRolePolicy(
		svc,
		roles.Auditor,
		"arn:aws:iam::aws:policy/ReadOnlyAccess",
	); err != nil {
		return nil, err
	}
	/*
		if err := awsiam.AttachRolePolicy(
			svc,
			roles.Auditor,
			"arn:aws:iam::aws:policy/SecurityAudit",
		); err != nil {
			return nil, err
		}
	*/
	return role, nil
}

// EnsureCloudWatchCrossAccountSharingRole creates the
// CloudWatch-CrossAccountSharingRole role in the account referenced by the
// given IAM client.  This role will be allowed to read CloudWatch logs and
// metrics via a service-linked role in the admin account(s).
func EnsureCloudWatchCrossAccountSharingRole(svc *iam.IAM, assumeRolePolicyDocument *policies.Document) (*awsiam.Role, error) {
	const roleName = "CloudWatch-CrossAccountSharingRole"
	role, err := awsiam.EnsureRole(
		svc,
		roleName,
		assumeRolePolicyDocument,
	)
	if err != nil {
		return nil, err
	}

	if err := awsiam.AttachRolePolicy(svc, roleName, "arn:aws:iam::aws:policy/job-function/ViewOnlyAccess"); err != nil {
		return nil, err
	}
	if err := awsiam.AttachRolePolicy(svc, roleName, "arn:aws:iam::aws:policy/AWSXrayReadOnlyAccess"); err != nil {
		return nil, err
	}
	if err := awsiam.AttachRolePolicy(svc, roleName, "arn:aws:iam::aws:policy/CloudWatchReadOnlyAccess"); err != nil {
		return nil, err
	}
	if err := awsiam.AttachRolePolicy(svc, roleName, "arn:aws:iam::aws:policy/CloudWatchAutomaticDashboardsAccess"); err != nil {
		return nil, err
	}

	return role, nil
}

func cannedPrincipals(svc *organizations.Organizations, bootstrapping bool) (
	canned struct{ AdminRolePrincipals, AuditorRolePrincipals, OrgAccountPrincipals *policies.Principal },
	err error,
) {
	var adminAccounts, allAccounts []*awsorgs.Account
	if adminAccounts, err = awsorgs.FindAccountsByDomain(svc, accounts.Admin); err != nil {
		return
	}
	if allAccounts, err = awsorgs.ListAccounts(svc); err != nil {
		return
	}
	var org *organizations.Organization
	org, err = awsorgs.DescribeOrganization(svc)
	if err != nil {
		return
	}

	if bootstrapping {
		canned.AdminRolePrincipals = &policies.Principal{AWS: make([]string, len(adminAccounts)+2)}     // +2 for the management account and its IAM user
		canned.AuditorRolePrincipals = &policies.Principal{AWS: make([]string, len(adminAccounts)*2+2)} // *2 for Administrator AND Auditor; +2 for the management account and its IAM user
		for i, account := range adminAccounts {
			canned.AdminRolePrincipals.AWS[i] = roles.Arn(aws.StringValue(account.Id), roles.Administrator)
			canned.AuditorRolePrincipals.AWS[i*2] = roles.Arn(aws.StringValue(account.Id), roles.Administrator)
			canned.AuditorRolePrincipals.AWS[i*2+1] = roles.Arn(aws.StringValue(account.Id), roles.Auditor)
		}
	} else {
		canned.AdminRolePrincipals = &policies.Principal{AWS: make([]string, len(adminAccounts)*3+2)}   // *3 for Administrator AND Intranet AND substrate-intranet; +2 for the management account and its IAM user
		canned.AuditorRolePrincipals = &policies.Principal{AWS: make([]string, len(adminAccounts)*4+2)} // *4 for Administrator AND Auditor AND Intranet AND substrate-intranet; +2 for the management account and its IAM user
		for i, account := range adminAccounts {
			canned.AdminRolePrincipals.AWS[i*3] = roles.Arn(aws.StringValue(account.Id), roles.Administrator)
			canned.AdminRolePrincipals.AWS[i*3+1] = roles.Arn(aws.StringValue(account.Id), roles.Intranet)
			canned.AdminRolePrincipals.AWS[i*3+2] = roles.Arn(aws.StringValue(account.Id), "substrate-intranet") // remove in 2021.12 (and in the comment above)
			canned.AuditorRolePrincipals.AWS[i*4] = roles.Arn(aws.StringValue(account.Id), roles.Administrator)
			canned.AuditorRolePrincipals.AWS[i*4+1] = roles.Arn(aws.StringValue(account.Id), roles.Auditor)
			canned.AuditorRolePrincipals.AWS[i*4+2] = roles.Arn(aws.StringValue(account.Id), roles.Intranet)
			canned.AuditorRolePrincipals.AWS[i*4+3] = roles.Arn(aws.StringValue(account.Id), "substrate-intranet") // remove in 2021.12 (and in the comment above)
		}
	}
	canned.AdminRolePrincipals.AWS[len(canned.AdminRolePrincipals.AWS)-2] = roles.Arn(
		aws.StringValue(org.MasterAccountId),
		roles.OrganizationAdministrator,
	)
	canned.AdminRolePrincipals.AWS[len(canned.AdminRolePrincipals.AWS)-1] = users.Arn(
		aws.StringValue(org.MasterAccountId),
		users.OrganizationAdministrator,
	)
	canned.AuditorRolePrincipals.AWS[len(canned.AuditorRolePrincipals.AWS)-2] = roles.Arn(
		aws.StringValue(org.MasterAccountId),
		roles.OrganizationAdministrator,
	)
	canned.AuditorRolePrincipals.AWS[len(canned.AuditorRolePrincipals.AWS)-1] = users.Arn(
		aws.StringValue(org.MasterAccountId),
		users.OrganizationAdministrator,
	)
	sort.Strings(canned.AdminRolePrincipals.AWS)   // to avoid spurious policy diffs
	sort.Strings(canned.AuditorRolePrincipals.AWS) // to avoid spurious policy diffs

	canned.OrgAccountPrincipals = &policies.Principal{AWS: make([]string, len(allAccounts))}
	for i, account := range allAccounts {
		canned.OrgAccountPrincipals.AWS[i] = aws.StringValue(account.Id)
	}
	sort.Strings(canned.OrgAccountPrincipals.AWS) // to avoid spurious policy diffs

	//log.Printf("%+v", canned)
	return
}
