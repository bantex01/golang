package locals3

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

// GetObject comment - pass in bucket and key objects - get out a string of s3 content data
func GetObject(bucket string, key string) string {

	svc := s3.New(session.New())
	req, err := svc.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		fmt.Printf("Unable to get object %v from bucket %v - err - %v\n", key, bucket, err)
		os.Exit(2)
	}

	fmt.Printf("Successful retrieval of object %v from bucket %v\n", key, bucket)
	var reader io.Reader
	reader = req.Body
	data, _ := ioutil.ReadAll(reader)
	var stringData string
	stringData = string(data[:])

	return stringData

}
