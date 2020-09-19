package main

import (
	"context"
	"fmt"
	"locals3"
	"oneke"
	"company/tf"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	sfxlambda "github.com/signalfx/lambda-go"
)

var handlerWrapper sfxlambda.HandlerWrapper

func handler(ctx context.Context, s3Event events.S3Event) {

	for _, record := range s3Event.Records {
		s3record := record.S3
		fmt.Printf("[%s - %s] Bucket: %s - Key: %s - Event_type: %s \n", record.EventSource, record.EventTime, s3record.Bucket.Name, s3record.Object.Key, record.EventName)

		//locals3.DetermineObject(record.EventName)

		// let's determine whether this is a put or delete operation and act accordingly
		switch record.EventName {

		case "ObjectRemoved:Delete":
			// If it's a delete op we need to take stack details from the notifcation and remove tests from 1ke. This is redundant for now.
			fmt.Println("Delete operation detected")

		case "ObjectCreated:Put":
			// If it's a put operation we need to determine whether 1ke has the test
			fmt.Println("Put operation detected")

			//Let's gather the file contents ready to parse

			var tfStateData string = locals3.GetObject(s3record.Bucket.Name, s3record.Object.Key)
			s := strings.Split(s3record.Object.Key, "/")
			fmt.Printf("Stack name: %v\n", s[2])
			stack := s[2]

			// Let's send this off to a terraform parse routine, we'll get back a map of tests to check (and possibly create)

			testData := tf.ParseJSON(tfStateData)

			// If we have no resources in our TF, then the TF state is empty, it means this is a stack delete

			if len(testData) == 1 && testData["DELETE"] == "YES" {
				fmt.Printf("We have instructions to delete any existing tests\n")
				stackTestData := oneke.GatherTestsForStack(stack)
				if len(stackTestData) == 0 {
					fmt.Printf("No existing tests found for %v, exiting\n", stack)
					break
				}
				deleteCounter := 0
				for url := range stackTestData {
					fmt.Printf("Delete test: %v - Type: %v - ID: %v", url, stackTestData[url]["testType"], stackTestData[url]["testID"])
					oneke.DeleteTest(stackTestData[url]["testType"].(string), stackTestData[url]["testID"].(int))
					deleteCounter++
				}

				/*dp := datapoint.Datapoint{
					Metric:     "db_calls",
					Value:      datapoint.NewIntValue(1),
					MetricType: datapoint.Counter,
					Dimensions: map[string]string{"db_name": "mysql1"},
				}
				handlerWrapper.SendDatapoints([]*datapoint.Datapoint{&dp})*/
				break
			}

			// If no search heads have been found, we'll move on. We aren't creating tests for single instances or IDM's at present (this may change in future)

			if len(testData) == 1 && testData["SEARCH_HEADS"] == "NONE_FOUND" {
				fmt.Printf("No search heads found, unable to determine instance type, let's cleanup any existing tests and exiting...\n")
				stackTestData := oneke.GatherTestsForStack(stack)
				if len(stackTestData) == 0 {
					fmt.Printf("No existing tests found for %v, exiting\n", stack)
					break
				}
				for url := range stackTestData {
					fmt.Printf("Delete test: %v - Type: %v - ID: %v", url, stackTestData[url]["testType"], stackTestData[url]["testID"])
					oneke.DeleteTest(stackTestData[url]["testType"].(string), stackTestData[url]["testID"].(int))
				}
				break
			}

			// If whitelisting has been found, we should delete any tests that were previously there

			if len(testData) == 1 && testData["WHITELISTING"] == "FOUND" {
				fmt.Printf("whitelisting found, checking for existing tests and if found, deleting...\n")
				stackTestData := oneke.GatherTestsForStack(stack)
				if len(stackTestData) == 0 {
					fmt.Printf("No existing tests found for %v, exiting\n", stack)
					break
				}
				for url := range stackTestData {
					fmt.Printf("Delete test: %v - Type: %v - ID: %v", url, stackTestData[url]["testType"], stackTestData[url]["testID"])
					oneke.DeleteTest(stackTestData[url]["testType"].(string), stackTestData[url]["testID"].(int))
				}
				break
			}

			// If we're here, the stack mathces our criteria for adding/deleting tests, let's get a list of tests from 1ke

			onekeTests := oneke.GatherAllTests()

			// We need to gather all tests for this particular stack as we need to also remove tests that are no longr required after we have checked on the tests that should be
			// there as reported by TFState

			stackTestData := oneke.GatherTestsForStack(stack)

			// We now have a map of 1ketests and a map of tests needed - let's check to see if the tests exist, if they do let's
			// leave as-is (if we deleted there would be a small outage as the 1ke tests don't come onboard for a few minutes), if they don't just create

			for testString, id := range testData {

				// testString looks like this 1ke-test~aqueduct-1ke-test.companyworks.lol

				s := strings.Split(testString, "~")
				//fmt.Printf("Stack name: %v\n", s[0])
				//stack := s[0]
				keyToCheckFor := "https://" + s[1] + "/en-US/account/login?loginType=company"

				fmt.Printf("Checking for existence of test: - %v\n", keyToCheckFor)
				if _, ok := onekeTests[keyToCheckFor]; ok {
					fmt.Printf("Test has been found: - %v - ID: %v - Test Type: %v - Test ID: %v - Test Name: %v\n", keyToCheckFor, id, onekeTests[keyToCheckFor]["testType"], onekeTests[keyToCheckFor]["testID"], onekeTests[keyToCheckFor]["testName"])
					_, ok := stackTestData[keyToCheckFor]
					if ok {
						delete(stackTestData, keyToCheckFor)
					}
				} else {
					if strings.Contains(testString, "stg.companycloud.com") || strings.Contains(testString, "companyworks.lol") {
						fmt.Printf("No test found but stg or dev environment detected, not actually creating test for %v\n", keyToCheckFor)
						//oneke.CreateTest(stack, "http-server", keyToCheckFor, id)
						_, ok := stackTestData[keyToCheckFor]
						if ok {
							delete(stackTestData, keyToCheckFor)
						}
					} else {
						fmt.Printf("No test found: %v - ID %v - Creating test at 1ke\n", keyToCheckFor, id)
						// We need to call our create 1ke test routine
						oneke.CreateTest(stack, "http-server", keyToCheckFor, id)
						_, ok := stackTestData[keyToCheckFor]
						if ok {
							delete(stackTestData, keyToCheckFor)
						}
					}

				}

			}

			// We should now have created the tests from the info provided by TF, let's do a final sweep of the tests that were there to see if we need to mop up

			if len(stackTestData) == 0 {
				fmt.Printf("No leftover tests, we appear to be in sync with TFstate\n")
			} else {
				fmt.Printf("We have leftover tests - these should be deleted to ensure we're in sync with TFstate\n")
				for url := range stackTestData {
					fmt.Printf("Test: %v - Type: %v - ID: %v\n", url, stackTestData[url]["testType"], stackTestData[url]["testID"])
					oneke.DeleteTest(stackTestData[url]["testType"].(string), stackTestData[url]["testID"].(int))
				}
			}

		}

	}

}

func main() {
	// Make the handler available for Remote Procedure Call by AWS Lambda

	handlerWrapper := sfxlambda.NewHandlerWrapper(lambda.NewHandler(handler))
	sfxlambda.Start(handlerWrapper)

	//lambda.Start(handler)
}
