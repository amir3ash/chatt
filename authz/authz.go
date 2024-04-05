package authz

import (
	"context"
	"io"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/authzed-go/v1"
	"github.com/authzed/grpcutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Conf struct {
	ApiUrl      string
	BearerToken string
}
type Authoriz struct {
	cli *authzed.Client
}

func NewAuthoriz(conf Conf) (*Authoriz, error) {
	systemCerts := grpcutil.WithInsecureBearerToken(conf.BearerToken)

	client, err := authzed.NewClient(
		conf.ApiUrl,
		systemCerts,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)

	if err != nil {
		return nil, err
	}

	return &Authoriz{client}, nil
}

// check a user that has relation to object
func (a Authoriz) Check(userId, relation, objType, objId string) (bool, error) {
	resp, err := a.cli.CheckPermission(context.Background(), &v1.CheckPermissionRequest{
		Resource:   &v1.ObjectReference{ObjectType: objType, ObjectId: objId},
		Permission: relation,
		Subject: &v1.SubjectReference{Object: &v1.ObjectReference{
			ObjectType: "user",
			ObjectId:   userId,
		}},
	})
	if err != nil {
		return false, err
	}

	return resp.Permissionship == v1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION, nil
}

func (a Authoriz) WhoHasRel(relation, objType, objId string) ([]string, error) {
	stream, err := a.cli.LookupSubjects(context.Background(), &v1.LookupSubjectsRequest{
		Resource:   &v1.ObjectReference{ObjectType: objType, ObjectId: objId},
		Permission: relation,
	})

	if err != nil {
		return nil, err
	}

	userids := make([]string, 0)
	for resp, err := stream.Recv(); err != io.EOF; resp, err = stream.Recv() {
		if err != nil {
			return nil, err
		}

		userids = append(userids, resp.Subject.SubjectObjectId)
	}
	return userids, nil
}

// List the objects of a particular type a user has access to.
func (a Authoriz) WhichObjsRelateToUser(userId, relation, objType string) ([]string, error) {
	stream, err := a.cli.LookupResources(context.Background(), &v1.LookupResourcesRequest{
		Subject: &v1.SubjectReference{Object: &v1.ObjectReference{
			ObjectType: "user",
			ObjectId:   userId,
		}},
		ResourceObjectType: objType,
		Permission:         relation,
	})
	if err != nil {
		return nil, err
	}

	objectIds := make([]string, 0)
	for resp, err := stream.Recv(); err != io.EOF; resp, err = stream.Recv() {
		if err != nil {
			return nil, err
		}

		objectIds = append(objectIds, resp.ResourceObjectId)
	}
	return objectIds, nil
}
