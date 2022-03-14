package resourcemanager

//Licensed under the Apache License, Version 2.0 (the "License");
//you may not use this file except in compliance with the License.
//You may obtain a copy of the License at
//
//http://www.apache.org/licenses/LICENSE-2.0
//
//Unless required by applicable law or agreed to in writing, software
//distributed under the License is distributed on an "AS IS" BASIS,
//WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//See the License for the specific language governing permissions and
//limitations under the License.
//
// Code generated by Alibaba Cloud SDK Code Generator.
// Changes may cause incorrect behavior and will be lost if the code is regenerated.

import (
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/responses"
)

// DetachControlPolicy invokes the resourcemanager.DetachControlPolicy API synchronously
func (client *Client) DetachControlPolicy(request *DetachControlPolicyRequest) (response *DetachControlPolicyResponse, err error) {
	response = CreateDetachControlPolicyResponse()
	err = client.DoAction(request, response)
	return
}

// DetachControlPolicyWithChan invokes the resourcemanager.DetachControlPolicy API asynchronously
func (client *Client) DetachControlPolicyWithChan(request *DetachControlPolicyRequest) (<-chan *DetachControlPolicyResponse, <-chan error) {
	responseChan := make(chan *DetachControlPolicyResponse, 1)
	errChan := make(chan error, 1)
	err := client.AddAsyncTask(func() {
		defer close(responseChan)
		defer close(errChan)
		response, err := client.DetachControlPolicy(request)
		if err != nil {
			errChan <- err
		} else {
			responseChan <- response
		}
	})
	if err != nil {
		errChan <- err
		close(responseChan)
		close(errChan)
	}
	return responseChan, errChan
}

// DetachControlPolicyWithCallback invokes the resourcemanager.DetachControlPolicy API asynchronously
func (client *Client) DetachControlPolicyWithCallback(request *DetachControlPolicyRequest, callback func(response *DetachControlPolicyResponse, err error)) <-chan int {
	result := make(chan int, 1)
	err := client.AddAsyncTask(func() {
		var response *DetachControlPolicyResponse
		var err error
		defer close(result)
		response, err = client.DetachControlPolicy(request)
		callback(response, err)
		result <- 1
	})
	if err != nil {
		defer close(result)
		callback(nil, err)
		result <- 0
	}
	return result
}

// DetachControlPolicyRequest is the request struct for api DetachControlPolicy
type DetachControlPolicyRequest struct {
	*requests.RpcRequest
	TargetId string `position:"Query" name:"TargetId"`
	PolicyId string `position:"Query" name:"PolicyId"`
}

// DetachControlPolicyResponse is the response struct for api DetachControlPolicy
type DetachControlPolicyResponse struct {
	*responses.BaseResponse
	RequestId string `json:"RequestId" xml:"RequestId"`
}

// CreateDetachControlPolicyRequest creates a request to invoke DetachControlPolicy API
func CreateDetachControlPolicyRequest() (request *DetachControlPolicyRequest) {
	request = &DetachControlPolicyRequest{
		RpcRequest: &requests.RpcRequest{},
	}
	request.InitWithApiInfo("ResourceManager", "2020-03-31", "DetachControlPolicy", "", "")
	request.Method = requests.POST
	return
}

// CreateDetachControlPolicyResponse creates a response to parse from DetachControlPolicy response
func CreateDetachControlPolicyResponse() (response *DetachControlPolicyResponse) {
	response = &DetachControlPolicyResponse{
		BaseResponse: &responses.BaseResponse{},
	}
	return
}
