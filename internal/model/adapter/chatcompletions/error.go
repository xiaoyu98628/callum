package chatcompletions

import (
	"errors"
	"net/http"

	"github.com/openai/openai-go/v3"

	"github.com/xiaoyu98628/callum/internal/model"
)

func requestID(response *http.Response) string {
	if response == nil {
		return ""
	}
	return response.Header.Get("x-request-id")
}

func wrapError(operation string, response *http.Response, cause error) error {
	statusCode := 0

	if apiError, ok := errors.AsType[*openai.Error](cause); ok {
		statusCode = apiError.StatusCode

		if response == nil {
			response = apiError.Response
		}
	}

	if response != nil {
		statusCode = response.StatusCode
	}

	return model.NewCallError(operation, statusCode, requestID(response), cause)
}
