package awsiam

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	iamv1 "github.com/aws/aws-sdk-go/service/iam"
	"github.com/src-bin/substrate/awscfg"
	"github.com/src-bin/substrate/awsiam/awsiamusers"
	"github.com/src-bin/substrate/awsutil"
	"github.com/src-bin/substrate/policies"
)

type (
	AccessKey         = types.AccessKey
	AccessKeyMetadata = types.AccessKeyMetadata
	User              = types.User
)

func CreateAccessKey(
	ctx context.Context,
	cfg *awscfg.Config,
	username string,
) (*AccessKey, error) {
	return awsiamusers.CreateAccessKey(ctx, cfg.IAM(), username)
}

func CreateAccessKeyV1(
	svc *iamv1.IAM,
	username string,
) (*iamv1.AccessKey, error) {
	out, err := svc.CreateAccessKey(&iamv1.CreateAccessKeyInput{
		UserName: aws.String(username),
	})
	if err != nil {
		return nil, err
	}
	//log.Printf("%+v", out)
	return out.AccessKey, nil
}

func CreateUser(
	ctx context.Context,
	cfg *awscfg.Config,
	username string,
) (*User, error) {
	return awsiamusers.CreateUser(ctx, cfg.IAM(), username)
}

func CreateUserV1(
	svc *iamv1.IAM,
	username string,
) (*iamv1.User, error) {
	out, err := svc.CreateUser(&iamv1.CreateUserInput{
		Tags:     tagsForV1(username),
		UserName: aws.String(username),
	})
	if err != nil {
		return nil, err
	}
	//log.Printf("%+v", out)
	time.Sleep(10e9) // give IAM time to become consistent (TODO do it gracefully)
	return out.User, nil
}

func DeleteAccessKey(
	ctx context.Context,
	cfg *awscfg.Config,
	username, accessKeyId string,
) error {
	return awsiamusers.DeleteAccessKey(ctx, cfg.IAM(), username, accessKeyId)
}

func DeleteAccessKeyV1(
	svc *iamv1.IAM,
	username, accessKeyId string,
) error {
	_, err := svc.DeleteAccessKey(&iamv1.DeleteAccessKeyInput{
		AccessKeyId: aws.String(accessKeyId),
		UserName:    aws.String(username),
	})
	return err
}

func DeleteAllAccessKeys(
	ctx context.Context,
	cfg *awscfg.Config,
	username string,
) error {
	return awsiamusers.DeleteAllAccessKeys(ctx, cfg.IAM(), username)
}

func DeleteAllAccessKeysV1(
	svc *iamv1.IAM,
	username string,
) error {
	meta, err := ListAccessKeysV1(svc, username)
	if err != nil {
		return err
	}
	for _, m := range meta {
		if err := DeleteAccessKeyV1(svc, username, aws.ToString(m.AccessKeyId)); err != nil {
			return err
		}
	}
	return nil
}

func EnsureUser(
	ctx context.Context,
	cfg *awscfg.Config,
	username string,
) (*User, error) {
	return awsiamusers.EnsureUser(ctx, cfg.IAM(), username)
}

func EnsureUserV1(
	svc *iamv1.IAM,
	username string,
) (*iamv1.User, error) {

	user, err := CreateUserV1(svc, username)
	if awsutil.ErrorCodeIs(err, EntityAlreadyExists) {
		user, err = GetUserV1(svc, username)
	}
	if err != nil {
		return nil, err
	}

	if _, err := svc.TagUser(&iamv1.TagUserInput{
		Tags:     tagsForV1(username),
		UserName: aws.String(username),
	}); err != nil {
		return nil, err
	}

	return user, nil
}

func EnsureUserWithPolicy(
	ctx context.Context,
	cfg *awscfg.Config,
	username string,
	doc *policies.Document,
) (*User, error) {
	return awsiamusers.EnsureUserWithPolicy(ctx, cfg.IAM(), username, doc)
}

func EnsureUserWithPolicyV1(
	svc *iamv1.IAM,
	username string,
	doc *policies.Document,
) (*iamv1.User, error) {

	user, err := EnsureUserV1(svc, username)
	if err != nil {
		return nil, err
	}

	// TODO attach the managed AdministratorAccess policy instead of inlining.
	docJSON, err := doc.Marshal()
	if err != nil {
		return nil, err
	}
	if _, err := svc.PutUserPolicy(&iamv1.PutUserPolicyInput{
		PolicyDocument: aws.String(docJSON),
		PolicyName:     aws.String(SubstrateManaged),
		UserName:       aws.String(username),
	}); err != nil {
		return nil, err
	}

	return user, nil
}

func GetUser(
	ctx context.Context,
	cfg *awscfg.Config,
	username string,
) (*User, error) {
	return awsiamusers.GetUser(ctx, cfg.IAM(), username)
}

func GetUserV1(
	svc *iamv1.IAM,
	username string,
) (*iamv1.User, error) {
	out, err := svc.GetUser(&iamv1.GetUserInput{
		UserName: aws.String(username),
	})
	if err != nil {
		return nil, err
	}
	//log.Printf("%+v", out)
	return out.User, nil
}

func ListAccessKeys(
	ctx context.Context,
	cfg *awscfg.Config,
	username string,
) ([]AccessKeyMetadata, error) {
	return awsiamusers.ListAccessKeys(ctx, cfg.IAM(), username)
}

func ListAccessKeysV1(
	svc *iamv1.IAM,
	username string,
) ([]*iamv1.AccessKeyMetadata, error) {
	out, err := svc.ListAccessKeys(&iamv1.ListAccessKeysInput{
		UserName: aws.String(username),
	})
	if err != nil {
		return nil, err
	}
	return out.AccessKeyMetadata, err
}
