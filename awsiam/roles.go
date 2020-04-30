package awsiam

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/src-bin/substrate/awsutil"
	"github.com/src-bin/substrate/policies"
)

func CreateRole(svc *iam.IAM, rolename string, principal *policies.Principal) (*iam.Role, error) {
	docJSON, err := assumeRolePolicyDocument(principal).JSON()
	if err != nil {
		return nil, err
	}
	in := &iam.CreateRoleInput{
		AssumeRolePolicyDocument: aws.String(docJSON),
		RoleName:                 aws.String(rolename),
		Tags:                     tagsFor(rolename),
	}
	out, err := svc.CreateRole(in)
	if err != nil {
		return nil, err
	}
	//log.Printf("%+v", out)
	return out.Role, nil
}

func EnsureRole(
	svc *iam.IAM,
	rolename string,
	principal *policies.Principal,
) (*iam.Role, error) {

	role, err := CreateRole(svc, rolename, principal)
	if awsutil.ErrorCodeIs(err, EntityAlreadyExists) {
		role, err = GetRole(svc, rolename)
	}
	if err != nil {
		return nil, err
	}

	if _, err := svc.TagRole(&iam.TagRoleInput{
		RoleName: aws.String(rolename),
		Tags:     tagsFor(rolename),
	}); err != nil {
		return nil, err
	}

	docJSON, err := assumeRolePolicyDocument(principal).JSON()
	if err != nil {
		return nil, err
	}
	if _, err := svc.UpdateAssumeRolePolicy(&iam.UpdateAssumeRolePolicyInput{
		PolicyDocument: aws.String(docJSON),
		RoleName:       aws.String(rolename),
	}); err != nil {
		return nil, err
	}

	return role, nil
}

func EnsureRoleWithPolicy(
	svc *iam.IAM,
	rolename string,
	principal *policies.Principal,
	doc *policies.Document,
) (*iam.Role, error) {

	role, err := EnsureRole(svc, rolename, principal)
	if err != nil {
		return nil, err
	}

	docJSON, err := doc.JSON()
	if err != nil {
		return nil, err
	}
	in := &iam.PutRolePolicyInput{
		PolicyDocument: aws.String(docJSON),
		PolicyName:     aws.String("Substrate"),
		RoleName:       aws.String(rolename),
	}
	if _, err := svc.PutRolePolicy(in); err != nil {
		return nil, err
	}

	return role, nil
}

func GetRole(svc *iam.IAM, rolename string) (*iam.Role, error) {
	in := &iam.GetRoleInput{
		RoleName: aws.String(rolename),
	}
	out, err := svc.GetRole(in)
	if err != nil {
		return nil, err
	}
	//log.Printf("%+v", out)
	return out.Role, nil
}

func assumeRolePolicyDocument(principal *policies.Principal) *policies.Document {
	return &policies.Document{
		Statement: []policies.Statement{
			policies.Statement{
				Principal: principal,
				Action:    []string{"sts:AssumeRole"},
			},
		},
	}
}
