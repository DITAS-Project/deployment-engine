package cloudsigma

import (
	"errors"
	"fmt"

	resty "gopkg.in/resty.v1"
)

const (
	ServersType       = "servers"
	DrivesType        = "drives"
	ServerStartAction = "start"
	ServerStopAction  = "stop"
)

type CloudSigmaError struct {
	Code        int    `json:"http_code"`
	Description string `json:"error_description"`
}

func (e CloudSigmaError) Error() string {
	return e.Description
}

type Client struct {
	httpClient *resty.Client
}

func NewClient(baseUrl string, username string, password string, debug bool) *Client {
	return &Client{
		httpClient: resty.New().SetRedirectPolicy(resty.FlexibleRedirectPolicy(20)).SetHostURL(baseUrl).SetBasicAuth(username, password).SetDebug(debug),
	}
}

func execute(request *resty.Request, path string, method string, result interface{}) error {

	//request.SetError(&CloudSigmaError{})

	if result != nil {
		request.SetResult(result)
	}

	response, errRequest := request.Execute(method, path)

	if errRequest != nil {
		return errRequest
	}

	if response.IsError() {
		return CloudSigmaError{
			Code:        response.StatusCode(),
			Description: response.String(),
		}
	}

	return nil
}

func getFirstObjectOfList(request *resty.Request, path string, method string) (ResourceType, error) {
	var listResponse RequestResponseType
	err := execute(request, path, method, &listResponse)
	if err == nil {
		if len(listResponse.Objects) > 0 {
			return listResponse.Objects[0], nil
		}
		return ResourceType{}, errors.New("Drive not found")

	}
	return ResourceType{}, err
}

func (c *Client) GetLibDrive(params map[string]string) (ResourceType, error) {
	return getFirstObjectOfList(c.httpClient.R().SetQueryParams(params), "/libdrives", resty.MethodGet)
}

func (c *Client) CloneDrive(uuid string, info *ResourceType) (ResourceType, error) {
	path := fmt.Sprintf("/libdrives/%s/action/?do=clone", uuid)
	request := c.httpClient.R()
	if info != nil {
		request = request.SetBody(info)
	}
	return getFirstObjectOfList(request, path, resty.MethodPost)
}

func (c *Client) GetDriveDetails(uuid string) (ResourceType, error) {
	var result ResourceType
	path := fmt.Sprintf("/drives/%s", uuid)
	err := execute(c.httpClient.R(), path, resty.MethodGet, &result)
	return result, err
}

func (c *Client) DeleteDrive(uuid string) error {
	path := fmt.Sprintf("/drives/%s/", uuid)
	err := execute(c.httpClient.R(), path, resty.MethodDelete, nil)
	return err
}

func (c *Client) CreateServers(servers RequestResponseType) (RequestResponseType, error) {
	var result RequestResponseType
	err := execute(c.httpClient.R().SetBody(servers), "/servers/", resty.MethodPost, &result)
	return result, err
}

func (c *Client) GetServerDetails(uuid string) (ResourceType, error) {
	var result ResourceType
	var path = fmt.Sprintf("/servers/%s", uuid)
	err := execute(c.httpClient.R(), path, resty.MethodGet, &result)
	return result, err
}

func (c *Client) ExecuteServerAction(uuid string, action string) (ActionResultType, error) {
	var result ActionResultType
	path := fmt.Sprintf("/servers/%s/action/?do=%s", uuid, action)
	err := execute(c.httpClient.R(), path, resty.MethodPost, &result)
	return result, err
}

func (c *Client) DeleteServerWithDrives(uuid string) error {
	path := fmt.Sprintf("/servers/%s/?recurse=all_drives", uuid)
	err := execute(c.httpClient.R(), path, resty.MethodDelete, nil)
	return err
}

func (c *Client) CreateTag(name string, resources []ResourceType) (ResourceType, error) {
	request := c.httpClient.R().SetBody(RequestResponseType{
		Objects: []ResourceType{
			ResourceType{
				Name:      name,
				Resources: resources,
			},
		},
	})
	return getFirstObjectOfList(request, "/tags/", resty.MethodPost)
}

func (c *Client) GetByTag(uuid string, resourceType string) (RequestResponseType, error) {
	var result RequestResponseType
	path := fmt.Sprintf("/tags/%s/%s/", uuid, resourceType)
	err := execute(c.httpClient.R(), path, resty.MethodGet, &result)
	return result, err
}

func (c *Client) GetTagInformation(uuid string) (ResourceType, error) {
	var result ResourceType
	path := fmt.Sprintf("/tag/%s/", uuid)
	err := execute(c.httpClient.R(), path, resty.MethodGet, &result)
	return result, err
}

func (c *Client) DeleteTag(uuid string) error {
	path := fmt.Sprintf("/tags/%s/", uuid)
	err := execute(c.httpClient.R(), path, resty.MethodDelete, nil)
	return err
}
