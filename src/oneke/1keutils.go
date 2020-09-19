package oneke

import (
	"bytes"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
)

// 1ke structs for tests

// onekeTestPayload comment
type onekeTestPayload struct {
	Test []onekeTest `json:"test"`
}

// onekeTest comment
type onekeTest struct {
	Enabled  int    `json:"enabled,omitempty"`
	TestID   int    `json:"testId,omitempty"`
	TestName string `json:"testName,omitempty"`
	TestType string `json:"type,omitempty"`
	URL      string `json:"url,omitempty"`
}

type onekeHTTPTestCreate struct {
	Interval            int          `json:"interval,omitempty"`
	Agents              []onekeAgent `json:"agents,omitempty"`
	TestName            string       `json:"testName,omitempty"`
	ContentRegex        string       `json:"contentRegex,omitempty"`
	URL                 string       `json:"url,omitempty"`
	AlertsEnabled       int          `json:"alertsEnabled"`
	BgpMeasurements     int          `json:"bgpMeasurements"`
	NetworkMeasurements int          `json:"networkMeasurements"`
	VerifyCertificate   int          `json:"verifyCertificate"`
}

type onekeAgent struct {
	AgentID int `json:"agentId,omitempty"`
}

func make1keRequest(reqType string, user string, token string, reqEndpoint string, reqPayload []byte) io.ReadCloser {

	fmt.Printf("make1keRequest called...\n")
	reqBody := bytes.NewBuffer(reqPayload)

	baseurl := "https://api.thousandeyes.com/v6" + reqEndpoint
	credsString := user + ":" + token
	sEnc := b64.StdEncoding.EncodeToString([]byte(credsString))

	req, err := http.NewRequest(reqType, baseurl, reqBody)
	if err != nil {
		fmt.Printf("Failed to create new HTTP request")
		os.Exit(2)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Basic "+sEnc)
	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		fmt.Printf("Client error - %v", resp)
		os.Exit(2)
	}

	//fmt.Printf("resp is %v", resp)
	return resp.Body

}

//DeleteTest comment
func DeleteTest(testType string, id int) {

	fmt.Printf("In delete test for type: %v - ID: %v\n", testType, id)

	stringid := strconv.Itoa(id)

	deleteString := "/tests/" + testType + "/" + stringid + "/delete.json"
	fmt.Printf("Delete string is %v\n", deleteString)

	user, token := Get1keToken()
	//fmt.Printf("User is %v and token is %v\n", user, token)
	if token != "" && user != "" {
		fmt.Printf("1ke API token and user retrieved successfully\n")
		fmt.Printf("Deleting Test: %v\n", id)
		resp := make1keRequest("POST", user, token, deleteString, nil)
		fmt.Printf("Response from delete request: %v\n", resp)
	}

}

// CreateTest comment
func CreateTest(stack string, testType string, testURL string, testID string) {

	fmt.Printf("CreateTest called...\n")
	switch testType {
	case "http-server":
		fmt.Printf("http-server test detected\n")

		testName := "stack=" + stack + " id=" + testID + " metric=web_check testname=web_check~" + testURL
		body := onekeHTTPTestCreate{
			Interval:            60,
			Agents:              []onekeAgent{{AgentID: 14410}},
			TestName:            testName,
			ContentRegex:        "someregex",
			URL:                 testURL,
			AlertsEnabled:       0,
			BgpMeasurements:     0,
			NetworkMeasurements: 0,
			VerifyCertificate:   0,
		}

		var jsonData []byte
		jsonData, err := json.Marshal(body)
		if err != nil {
			log.Println(err)
		}

		fmt.Println(string(jsonData))

		user, token := Get1keToken()
		//fmt.Printf("User is %v and token is %v\n", user, token)
		if token != "" && user != "" {
			fmt.Printf("1ke API token and user retrieved successfully\n")
			fmt.Printf("Creating Test: %v\n", testName)
			make1keRequest("POST", user, token, "/tests/http-server/new.json", jsonData)
		}

	case "other-test":
		fmt.Printf("other test\n")

	}

}

// GatherTestsForStack comment
func GatherTestsForStack(stack string) map[string]map[string]interface{} {

	stackTests := make(map[string]map[string]interface{})
	allTests := GatherAllTests()

	for url := range allTests {
		//fmt.Printf("URL - %v - Deets - %v\n", url, deets)
		regexString := ".+" + stack + "\\.(?:stg|companyworks|companycloud)?\\.(?:companycloud|com|lol)?"
		//matched, err := regexp.MatchString("(.+)something\\.(?:stg|companyworks|companycloud)?\\.(?:companycloud|com|lol)?", url)
		matched, err := regexp.MatchString(regexString, url)
		if matched {
			inner, ok := stackTests[url]
			if !ok {
				inner = make(map[string]interface{})
				stackTests[url] = inner
			}

			stackTests[url]["testName"] = allTests[url]["testName"]
			stackTests[url]["testType"] = allTests[url]["testType"]
			stackTests[url]["testID"] = allTests[url]["testID"]
		}
		if err != nil {
			fmt.Printf("Regex Issue, %v", err)
		}
	}

	return stackTests

}

// GatherAllTests comment
func GatherAllTests() map[string]map[string]interface{} {

	fmt.Printf("GatherAllTests called...\n")
	// let's get user and token to make API call

	user, token := Get1keToken()
	if token != "" && user != "" {
		fmt.Printf("1ke API token and user retrieved successfully\n")
	}

	requestBody := make1keRequest("GET", user, token, "/tests", nil)

	clientByteValue, _ := ioutil.ReadAll(requestBody)
	var clientResults onekeTestPayload
	if err := json.Unmarshal(clientByteValue, &clientResults); err != nil {
		panic(err)
	}

	onekeTestData := make(map[string]map[string]interface{})

	for _, test := range clientResults.Test {
		inner, ok := onekeTestData[test.URL]
		if !ok {
			inner = make(map[string]interface{})
			onekeTestData[test.URL] = inner
		}
		onekeTestData[test.URL]["testName"] = test.TestName
		onekeTestData[test.URL]["testType"] = test.TestType
		onekeTestData[test.URL]["testID"] = test.TestID
	}

	/*
		testMap looks like:
		onekeTestData["https://something.companycloud.com/en-US/account/login?loginType=company"]["testName"] = "stack=something id=standard metric=web_check testname=web_check~https://something.companycloud.com/en-US/account/login?loginType=company"
		onekeTestData["https://something.companycloud.com/en-US/account/login?loginType=company"]["testID"] = "123567"
		onekeTestData["https://something.companycloud.com/en-US/account/login?loginType=company"]["testType"] = "http-server"
	*/

	return onekeTestData
}

// Get1keToken comment
func Get1keToken() (string, string) {

	fmt.Println("Get1keToken called...")
	secretName := "some-api"
	region := "us-west-2"
	fmt.Println("us-west-2 region set for secret management...")

	//Create a Secrets Manager client
	svc := secretsmanager.New(session.New(), aws.NewConfig().WithRegion(region))
	input := &secretsmanager.GetSecretValueInput{
		SecretId:     aws.String(secretName),
		VersionStage: aws.String("AWSCURRENT"), // VersionStage defaults to AWSCURRENT if unspecified
	}

	result, err := svc.GetSecretValue(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case secretsmanager.ErrCodeDecryptionFailure:
				// Secrets Manager can't decrypt the protected secret text using the provided KMS key.
				fmt.Println(secretsmanager.ErrCodeDecryptionFailure, aerr.Error())

			case secretsmanager.ErrCodeInternalServiceError:
				// An error occurred on the server side.
				fmt.Println(secretsmanager.ErrCodeInternalServiceError, aerr.Error())

			case secretsmanager.ErrCodeInvalidParameterException:
				// You provided an invalid value for a parameter.
				fmt.Println(secretsmanager.ErrCodeInvalidParameterException, aerr.Error())

			case secretsmanager.ErrCodeInvalidRequestException:
				// You provided a parameter value that is not valid for the current state of the resource.
				fmt.Println(secretsmanager.ErrCodeInvalidRequestException, aerr.Error())

			case secretsmanager.ErrCodeResourceNotFoundException:
				// We can't find the resource that you asked for.
				fmt.Println(secretsmanager.ErrCodeResourceNotFoundException, aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}

	}

	// Decrypts secret using the associated KMS CMK.

	var secretString string
	if result.SecretString != nil {
		secretString = *result.SecretString
	}

	var secretJSON map[string]string
	json.Unmarshal([]byte(secretString), &secretJSON)

	var onekeUser string
	var onekeToken string

	for fonekeUser, fonekeToken := range secretJSON {
		onekeUser = fonekeUser
		onekeToken = fonekeToken
	}

	return onekeUser, onekeToken

}
