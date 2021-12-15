package vaultapi

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/skyflowapi/skyflow-go/errors"
)

type getByIdApi struct {
	configuration Configuration
	records       GetByIdInput
	token         string
}

func (g *getByIdApi) get() (map[string]interface{}, *errors.SkyflowError) {
	err := g.validateRecords(g.records)
	if err != nil {
		return nil, err
	}
	res, err := g.doRequest()
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (g *getByIdApi) validateRecords(records GetByIdInput) *errors.SkyflowError {
	if len(records.Records) == 0 {
		return nil
	}
	for i := 0; i < len(records.Records); i++ {
		singleRecord := records.Records[0]
		if singleRecord.Table == "" {
			return nil
		} else if len(singleRecord.Ids) == 0 {

		} else if singleRecord.Redaction != REDACTED || singleRecord.Redaction != MASKED || singleRecord.Redaction != PLAIN_TEXT {

		}
		for i := 0; i < len(singleRecord.Ids); i++ {
			if singleRecord.Ids[0] == "" {

			}
		}

	}
	return nil
}

func (g *getByIdApi) doRequest() (map[string]interface{}, *errors.SkyflowError) {

	var wg = sync.WaitGroup{}
	var finalSuccess []interface{}
	var finalError []map[string]interface{}
	for i := 0; i < len(g.records.Records); i++ {
		wg.Add(1)
		singleRecord := g.records.Records[i]
		requestUrl := fmt.Sprintf("%s/v1/vaults/%s/%s", g.configuration.VaultURL, g.configuration.VaultID, singleRecord.Table)
		url1, err := url.Parse(requestUrl)
		v := url.Values{}
		for j := 0; j < len(singleRecord.Ids); j++ {
			v.Add("skyflow_ids", singleRecord.Ids[j])
		}
		v.Add("redaction", string(singleRecord.Redaction))
		url1.RawQuery = v.Encode()
		if err == nil {
			request, _ := http.NewRequest(
				"GET",
				url1.String(),
				strings.NewReader(""),
			)
			bearerToken := fmt.Sprintf("Bearer %s", g.token)
			request.Header.Add("Authorization", bearerToken)
			res, err := http.DefaultClient.Do(request)
			if err != nil {
				fmt.Println("error from server: ", err)
			}
			data, _ := ioutil.ReadAll(res.Body)
			res.Body.Close()
			var result map[string]interface{}
			err = json.Unmarshal(data, &result)
			if err != nil {
				fmt.Println(err)
				//return nil, errors.NewSkyflowError(errors.ErrorCodesEnum(DEFAULT), errors.INVALID_FIELD)
			} else {
				errorResult := result["error"]
				if errorResult != nil {
					var generatedError = (errorResult).(map[string]interface{})
					fmt.Println(generatedError)
					var error = make(map[string]interface{})
					//var skyflowError = errors.NewSkyflowError("404", (generatedError["message"]).(string))
					error["error"] = generatedError["message"]
					error["ids"] = singleRecord.Ids
					finalError = append(finalError, error)

				} else {
					records := (result["records"]).([]interface{})
					new := make(map[string]interface{})
					for k := 0; k < len(records); k++ {
						single := (records[k]).(map[string]interface{})
						fields := (single["fields"]).(map[string]interface{})
						fields["id"] = fields["skyflow_id"]
						delete(fields, "skyflow_id")
						new["fields"] = fields
						new["table"] = singleRecord.Table
						finalSuccess = append(finalSuccess, new)
					}
				}

			}
		}
		wg.Done()
	}

	wg.Wait()
	var finalRecord = make(map[string]interface{})
	finalRecord["success"] = finalSuccess
	finalRecord["errors"] = finalError
	return finalRecord, nil
}
