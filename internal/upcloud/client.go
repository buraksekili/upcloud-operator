package upcloud

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/client"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/request"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/service"
	"github.com/buraksekili/upcloud-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/jinzhu/copier"
)

type Client struct {
	svc    *service.Service
	Logger logr.Logger
}

func NewClient() (Client, error) {
	username := os.Getenv("UPCLOUDOPERATOR_USERNAME")
	if username == "" {
		return Client{}, errors.New("failed to parse UpCloud username, please specify UPCLOUDOPERATOR_USERNAME")
	}

	pw := os.Getenv("UPCLOUDOPERATOR_PASSWORD")
	if pw == "" {
		return Client{}, errors.New(
			"failed to parse UpCloud username password, please specify UPCLOUDOPERATOR_PASSWORD",
		)
	}

	svc := service.New(client.New(username, pw))

	_, err := svc.GetAccount(context.Background())
	if err != nil {
		// `upcloud.Problem` is the error object returned by all of the `Service` methods.
		//  You can differentiate between generic connection errors (like the API not being reachable) and service errors, which are errors returned in the response body by the API;
		//	this is useful for gracefully recovering from certain types of errors;
		var problem *upcloud.Problem

		if errors.As(err, &problem) {
			fmt.Println(problem.Status)        // HTTP status code returned by the API
			fmt.Print(problem.Title)           // Short, human-readable description of the problem
			fmt.Println(problem.CorrelationID) // Unique string that identifies the request that caused the problem; note that this field is not always populated
			fmt.Println(problem.InvalidParams) // List of invalid request parameters

			for _, invalidParam := range problem.InvalidParams {
				fmt.Println(invalidParam.Name)   // Path to the request field that is invalid
				fmt.Println(invalidParam.Reason) // Human-readable description of the problem with that particular field
			}

			// You can also check against the specific api error codes to programatically react to certain situations.
			// Base `upcloud` package exports all the error codes that API can return.
			// You can check which error code is return in which situation in UpCloud API docs -> https://developers.upcloud.com/1.3
			if problem.ErrorCode() == upcloud.ErrCodeResourceAlreadyExists {
				fmt.Println("Looks like we don't need to create this")
			}

			// `upcloud.Problem` implements the Error interface, so you can also just use it as any other error
			return Client{}, fmt.Errorf("we got an error from the UpCloud API: %w", problem)
		} else {
			// This means you got an error, but it does not come from the API itself. This can happen, for example, if you have some connection issues,
			// or if the UpCloud API is unreachable for some other reason
			fmt.Println("We got a generic error!")
			return Client{}, err
		}
	}

	return Client{svc: svc}, nil
}

func (c *Client) GetVM(ctx context.Context, UUID string) (*upcloud.ServerDetails, error) {
	return c.svc.GetServerDetails(ctx, &request.GetServerDetailsRequest{UUID: UUID})
}

func (c *Client) Exists(ctx context.Context, UUID string) (*upcloud.ServerDetails, bool) {
	if UUID == "" {
		return nil, false
	}

	sd, err := c.GetVM(ctx, UUID)
	if err == nil {
		return sd, true
	}

	return nil, false
}

func (c *Client) CreateVM(ctx context.Context, desired *v1alpha1.VM) (*upcloud.ServerDetails, error) {
	if sd, exists := c.Exists(ctx, desired.Status.UUID); exists {
		return sd, nil
	}

	// Create the server
	serverDetails, err := c.svc.CreateServer(ctx, createServerReq(desired))
	if err != nil {
		c.Logger.Error(err, "failed to create VM")
		return nil, err
	}

	c.Logger.Info("Server has created successfully", "UUID", serverDetails.UUID, "Title", serverDetails.Title)

	return serverDetails, err
}

func (c *Client) StopVM(ctx context.Context, desired *v1alpha1.VM) (*upcloud.ServerDetails, error) {
	newServerDetails, err := c.svc.StopServer(ctx, &request.StopServerRequest{UUID: desired.Status.UUID})
	if err != nil {
		return nil, err
	}

	return newServerDetails, nil
}

func (c *Client) DeleteVM(ctx context.Context, desired *v1alpha1.VM) error {
	return c.svc.DeleteServer(ctx, &request.DeleteServerRequest{UUID: desired.Status.UUID})
}

func (c *Client) CreateOrUpdateVM(ctx context.Context, vm *v1alpha1.VM) (*upcloud.ServerDetails, error) {
	currentServerDetails, exists := c.Exists(ctx, vm.Status.UUID)
	if !exists {
		return c.CreateVM(ctx, vm)
	}

	if currentServerDetails.State == upcloud.ServerStateMaintenance {
		return currentServerDetails, nil
	}

	// Create the server
	serverDetails, err := c.svc.ModifyServer(ctx, updateServerReq(vm))
	if err != nil {
		c.Logger.Error(err, "failed to update VM")
		return nil, err
	}

	c.Logger.Info("Server has been updated successfully", "UUID", serverDetails.UUID, "Title", serverDetails.Title)

	return serverDetails, err
}

func (c *Client) StartVM(ctx context.Context, vm *v1alpha1.VM) (*upcloud.ServerDetails, error) {
	return c.svc.StartServer(ctx, &request.StartServerRequest{UUID: vm.Status.UUID})
}

func createServerReq(desired *v1alpha1.VM) *request.CreateServerRequest {
	r := &request.CreateServerRequest{}
	copier.Copy(r, desired.Spec.Server)
	return r
}

func updateServerReq(desired *v1alpha1.VM) *request.ModifyServerRequest {
	r := &request.ModifyServerRequest{}
	copier.Copy(r, desired.Spec.Server)
	r.UUID = desired.Status.UUID
	r.Zone = ""
	return r
}
