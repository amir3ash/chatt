package authz

import (
	"context"
	"fmt"
	"io"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/authzed-go/v1"
	"github.com/authzed/grpcutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Tuple struct{ UserId, Relation, ObjType, ObjId string }

type Conf struct {
	ApiUrl      string
	BearerToken string
}
type Authoriz struct {
	cli *authzed.Client
}

func NewAuthoriz(cli *authzed.Client) *Authoriz {
	return &Authoriz{cli}
}
func NewInsecureAuthZedCli(conf Conf) (*authzed.Client, error) {
	systemCerts := grpcutil.WithInsecureBearerToken(conf.BearerToken)

	client, err := authzed.NewClient(
		conf.ApiUrl,
		systemCerts,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)

	if err != nil {
		return nil, err
	}

	return client, nil
}

// check a user that has relation to object
func (a Authoriz) Check(ctx context.Context, userId, relation, objType, objId string) (bool, error) {
	resp, err := a.cli.CheckPermission(ctx, &v1.CheckPermissionRequest{
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

func (a Authoriz) WhoHasRel(ctx context.Context, relation, objType, objId string) ([]string, error) {
	stream, err := a.cli.LookupSubjects(ctx, &v1.LookupSubjectsRequest{
		Resource:          &v1.ObjectReference{ObjectType: objType, ObjectId: objId},
		Permission:        relation,
		SubjectObjectType: "user",
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
func (a Authoriz) WhichObjsRelateToUser(ctx context.Context, userId, relation, objType string) ([]string, error) {
	stream, err := a.cli.LookupResources(ctx, &v1.LookupResourcesRequest{
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

func (a Authoriz) BulkCheck(ctx context.Context, tuples []Tuple) ([]bool, []error) {
	requests := make([]*v1.CheckBulkPermissionsRequestItem, len(tuples))
	allowed := make([]bool, len(tuples))
	errs := make([]error, len(tuples))

	for i, v := range tuples {
		requests[i] = &v1.CheckBulkPermissionsRequestItem{
			Resource:   &v1.ObjectReference{ObjectType: v.ObjType, ObjectId: v.ObjId},
			Permission: v.Relation,
			Subject: &v1.SubjectReference{Object: &v1.ObjectReference{
				ObjectType: "user",
				ObjectId:   v.UserId,
			}},
		}
	}
	resp, err := a.cli.CheckBulkPermissions(ctx, &v1.CheckBulkPermissionsRequest{
		Items: requests,
	})
	if err != nil {
		for i := range errs {
			errs[i] = err
		}
		return nil, errs
	}

	for i, v := range resp.Pairs {
		err := v.GetError()
		if err != nil {
			errs[i] = fmt.Errorf("authzed bulk err code=%d, mesg=%s", err.GetCode(), err.GetMessage())
		} else {
			allowed[i] = v.GetItem().Permissionship == v1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION
		}
	}
	return allowed, errs
}
