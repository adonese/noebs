package ebs_fields

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

var log = logrus.New()

// EBSHttpClient the client to interact with EBS
func EBSHttpClient(url string, req []byte) (int, EBSParserFields, error) {

	verifyTLS := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	ebsClient := http.Client{
		Timeout:   3 * 30 * time.Second,
		Transport: verifyTLS,
	}

	log.Printf("EBS url is: %v", url)
	log.Printf("our request to EBS: %v", string(req))
	reqBuffer := bytes.NewBuffer(req)

	var ebsGenericResponse EBSParserFields

	reqHandler, err := http.NewRequest(http.MethodPost, url, reqBuffer)

	if err != nil {
		fmt.Println(err.Error())
		log.WithFields(logrus.Fields{
			"code": err.Error(),
		}).Error("Error in establishing connection to the host")
		return 500, ebsGenericResponse, err
	}
	reqHandler.Header.Set("Content-Type", "application/json")

	ebsResponse, err := ebsClient.Do(reqHandler)
	if err != nil {
		log.WithFields(logrus.Fields{
			"code": err.Error(),
		}).Error("Error in establishing connection to the host")
		return http.StatusGatewayTimeout, ebsGenericResponse, EbsGatewayConnectivityErr
	}

	defer ebsResponse.Body.Close()
	responseBody, err := io.ReadAll(ebsResponse.Body)
	if err != nil {
		log.WithFields(logrus.Fields{
			"code": err.Error(),
		}).Error("Error reading ebs response")
		return http.StatusInternalServerError, ebsGenericResponse, EbsGatewayConnectivityErr
	}
	var c CacheCards
	var isValid = true
	c.Pan = getPan(req)

	log.Printf("ebs_raw: %s", string(responseBody))
	if !strings.Contains(ebsResponse.Header.Get("Content-Type"), "application/json") {
		log.WithFields(logrus.Fields{
			"code":    "wrong content type parsed",
			"details": ebsResponse.Header.Get("Content-Type"),
		}).Error("ebs response content type is not application/json")
		return http.StatusInternalServerError, ebsGenericResponse, ContentTypeErr
	}
	var tmpRes IPINResponse
	if err := json.Unmarshal(responseBody, &ebsGenericResponse); err == nil {
		if ebsGenericResponse.ResponseCode == INVALIDCARD {
			isValid = false
		}
		c.IsValid = &isValid
		EBSRes <- c
		if ebsGenericResponse.ResponseCode == 0 || strings.Contains(ebsGenericResponse.ResponseMessage, "Success") {
			return http.StatusOK, ebsGenericResponse, nil
		} else {
			return http.StatusBadGateway, ebsGenericResponse, errors.New(ebsGenericResponse.ResponseMessage)
		}
	} else {
		// there is an error in handling the incoming EBS's ebsResponse
		// log the err here please
		log.WithFields(logrus.Fields{
			"code":         err.Error(),
			"all_response": string(responseBody),
			"ebs_fields":   ebsGenericResponse,
		}).Info("ebs response transaction")
		if strings.Contains(err.Error(), " EBSParserFields.tranDateTime of type string") {
			json.Unmarshal(responseBody, &tmpRes)
			if tmpRes.ResponseCode == 0 || strings.Contains(tmpRes.ResponseMessage, "Success") {
				return http.StatusOK, tmpRes.newResponse(), nil
			} else {
				return http.StatusBadGateway, ebsGenericResponse, errors.New(ebsGenericResponse.ResponseMessage)
			}
		}
		return http.StatusInternalServerError, ebsGenericResponse, err
	}

}

type IPINResponse struct {
	UUID            string `json:"UUID"`
	TranDateTime    int    `json:"tranDateTime"`
	ResponseMessage string `json:"responseMessage"`
	ResponseStatus  string `json:"responseStatus"`
	PubKeyValue     string `json:"pubKeyValue"`
	ResponseCode    int64  `json:"responseCode"`
	Pan             string `json:"pan"`
	ExpDate         string `json:"expDate"`
	Username        string `json:"userName"`
}

// newResponse the
func (i IPINResponse) newResponse() EBSParserFields {
	var res EBSResponse
	res.ResponseCode = int(i.ResponseCode)
	res.ResponseMessage = i.ResponseMessage
	res.PubKeyValue = i.PubKeyValue
	res.TranDateTime = strconv.Itoa(i.TranDateTime)
	res.UUID = i.UUID
	res.PAN = i.Pan
	res.ExpDate = i.ExpDate
	return EBSParserFields{EBSResponse: res}
}

var EBSRes = make(chan CacheCards)

func getPan(data []byte) string {
	var d map[string]any
	json.Unmarshal(data, &d)
	if res, ok := d["PAN"].(string); ok {
		return res
	}
	return ""
}

var (
	INVALIDPIN   = 53
	SUCCESS      = 0
	INVALIDCARD  = 52
	ROUTINGERROR = 72
)

type Configs struct {
	DB *gorm.DB
}
