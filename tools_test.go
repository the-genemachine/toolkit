package toolkit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
)

type RoundTripFunc func(req *http.Request) *http.Response

func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func NewTestClient(fn RoundTripFunc) *http.Client {
	return &http.Client{
		Transport: fn,
	}
}

func TestTools_PushJSONToremote(t *testing.T) {
	client := NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString("OK")),
			Header:     make(http.Header),
		}
	})

	var testTools Tools
	var foo struct {
		Bar string `json:"bar"`
	}
	foo.Bar = "bar"

	_, _, err := testTools.PushJSONToRemote("http://example.com", foo, client)
	if err != nil {
		t.Error("failed to call remote uri", err)
	}
}

func TestTools_RandomString(t *testing.T) {
	var tools Tools

	s := tools.RandomString(10)
	if len(s) != 10 {
		t.Error("Random string length is not 10")
	}
}

var uploadTests = []struct {
	name          string
	allowedTypes  []string
	renameFiles   bool
	errorExpected bool
}{
	{name: "allowed no rename", allowedTypes: []string{"image/jpeg", "image/png", "image/gif"}, renameFiles: false, errorExpected: false},
	{name: "allowed rename", allowedTypes: []string{"image/jpeg", "image/png", "image/gif"}, renameFiles: true, errorExpected: false},
	{name: "not allowed", allowedTypes: []string{"image/jpeg", "image/gif"}, renameFiles: false, errorExpected: true},
}

func TestTools_UploadFiles(t *testing.T) {
	for _, e := range uploadTests {
		pr, pw := io.Pipe()
		writer := multipart.NewWriter(pw)
		wg := sync.WaitGroup{}
		wg.Add(1)

		go func() {
			defer writer.Close()
			defer wg.Done()

			part, err := writer.CreateFormFile("file", "./testdata/img.png")
			if err != nil {
				t.Error(err)
			}

			f, err := os.Open("./testdata/img.png")
			if err != nil {
				t.Error(err)
			}
			defer f.Close()

			img, _, err := image.Decode(f)
			if err != nil {
				t.Error(err)
			}

			err = png.Encode(part, img)
			if err != nil {
				t.Error(err)
			}

		}()
		// read from the pipe with recieves data
		request := httptest.NewRequest("POST", "/", pr)
		request.Header.Set("Content-Type", writer.FormDataContentType())

		var testTools Tools
		testTools.AllowedFileTypes = e.allowedTypes

		uploadedFiles, err := testTools.UploadFiles(request, "./testdata/uploads/", e.renameFiles)
		if err != nil && !e.errorExpected {
			t.Error(err)
		}

		if !e.errorExpected {
			if _, err := os.Stat(fmt.Sprintf("./testdata/uploads/%s", uploadedFiles[0].NewFileName)); os.IsNotExist(err) {
				t.Errorf("%s : expected file to exist: %s", e.name, err.Error())
			}

			_ = os.Remove(fmt.Sprintf("./testdata/uploads/%s", uploadedFiles[0].NewFileName))
		}
		if !e.errorExpected && err != nil {
			t.Errorf("%s : error expected but not received", e.name)
		}
		wg.Wait()
	}
}

func TestTools_UploadOneFile(t *testing.T) {

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		defer writer.Close()

		part, err := writer.CreateFormFile("file", "./testdata/img.png")
		if err != nil {
			t.Error(err)
		}

		f, err := os.Open("./testdata/img.png")
		if err != nil {
			t.Error(err)
		}
		defer f.Close()

		img, _, err := image.Decode(f)
		if err != nil {
			t.Error(err)
		}

		err = png.Encode(part, img)
		if err != nil {
			t.Error(err)
		}

	}()

	// read from the pipe with recieves data
	request := httptest.NewRequest("POST", "/", pr)
	request.Header.Set("Content-Type", writer.FormDataContentType())

	var testTools Tools

	uploadedFile, err := testTools.UploadOneFile(request, "./testdata/uploads/", true)
	if err != nil {
		t.Error(err)
	}

	if _, err := os.Stat(fmt.Sprintf("./testdata/uploads/%s", uploadedFile.NewFileName)); os.IsNotExist(err) {
		t.Errorf("expected file to exist: %s", err.Error())
	}

	_ = os.Remove(fmt.Sprintf("./testdata/uploads/%s", uploadedFile.NewFileName))

}
func TestTools_CreateDirIfNotExist(t *testing.T) {
	var testTools Tools

	err := testTools.CreateDirIfNotExist("./testdata/testdir")
	if err != nil {
		t.Error(err)
	}

	if _, err := os.Stat("./testdata/testdir"); os.IsNotExist(err) {
		t.Errorf("expected dir to exist: %s", err.Error())
	}

	_ = os.Remove("./testdata/testdir")
}

var slugtests = []struct {
	name          string
	s             string
	expected      string
	errorExpected bool
}{
	{name: "valid string", s: "now is the time", expected: "now-is-the-time", errorExpected: false},
	{name: "empty string", s: "", expected: "", errorExpected: true},
	{name: "complex string", s: "Now is the time for all GOOD men! + fish & such &^123", expected: "now-is-the-time-for-all-good-men-fish-such-123", errorExpected: false},
	{name: "japanes string", s: "ハローワールド", expected: "", errorExpected: true},
	{name: "japanes string and roman characters", s: "hello world : ハローワールド", expected: "hello-world", errorExpected: false},
}

func TestTools_Slugify(t *testing.T) {
	var testTools Tools

	for _, e := range slugtests {
		slug, err := testTools.Slugify(e.s)
		if err != nil && !e.errorExpected {
			t.Errorf("%s : error recieved but not expected : %s", e.name, err.Error())
		}
		if !e.errorExpected && slug != e.expected {
			t.Errorf("%s : expected %s got %s", e.name, e.expected, slug)
		}
	}
}

func TestTools_DownloadStaticFile(t *testing.T) {
	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)

	var testTool Tools

	testTool.DownloadStaticFile(rr, req, "./testdata", "HiMatic-7s.jpg", "Classic-Car.jpg")

	res := rr.Result()
	defer res.Body.Close()

	/* if res.Header["Content-Length"][0] != "79064" {
		t.Error("wrong content length of", res.Header["Content-Length"][0])
	} */

	if res.Header["Content-Disposition"][0] != "attachment; filename=\"Classic-Car.jpg\"" {
		t.Error("wrong content disposition")
	}

	_, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Error(err)
	}

}

var jsonTests = []struct {
	name          string
	json          string
	errorExpected bool
	maxSize       int
	allowUnknown  bool
}{
	{name: "valid json", json: `{"foo":"bar"}`, errorExpected: false, maxSize: 1024, allowUnknown: false},
	{name: "invalid json : badly formatted", json: `{"foo":}`, errorExpected: true, maxSize: 1024, allowUnknown: false},
	{name: "invalid json : incorrect type", json: `{"foo": 1}`, errorExpected: true, maxSize: 1024, allowUnknown: false},
	{name: "invalid json : two json files", json: `{"foo": "1"}{"alpha" : "beta"}`, errorExpected: true, maxSize: 1024, allowUnknown: false},
	{name: "invalid json : empty body", json: ``, errorExpected: true, maxSize: 1024, allowUnknown: false},
	{name: "invalid json : syntax error", json: `{"foo" : 1"} `, errorExpected: true, maxSize: 1024, allowUnknown: false},
	{name: "invalid json : unknown field", json: `{"fooo" : 1"} `, errorExpected: true, maxSize: 1024, allowUnknown: false},
	{name: "invalid json : allow unknown fields", json: `{"fooo" : "1"} `, errorExpected: false, maxSize: 1024, allowUnknown: true},
	{name: "invalid json : missing field", json: `{foo : "1"} `, errorExpected: true, maxSize: 1024, allowUnknown: true},
	{name: "invalid json : file too large", json: `{foo : "bar"} `, errorExpected: true, maxSize: 3, allowUnknown: true},
	{name: "invalid json : not json", json: `Hello World `, errorExpected: true, maxSize: 1024, allowUnknown: true},
}

// write a table test
func TestTools_ReadJSONFile(t *testing.T) {
	var testTool Tools

	for _, e := range jsonTests {
		testTool.MaxJSONSize = e.maxSize

		testTool.AllowUnknownFields = e.allowUnknown

		var decodedJSON struct {
			Foo string `json:"foo"`
		}

		req, err := http.NewRequest("POST", "/", bytes.NewReader([]byte(e.json)))
		if err != nil {
			t.Log("Error : ", err)
		}

		// create a new recorder
		rr := httptest.NewRecorder()

		// call the function
		err = testTool.ReadJSONFile(rr, req, &decodedJSON)

		if e.errorExpected && err == nil {
			t.Errorf("%s : error expected but not received", e.name)
		}

		if !e.errorExpected && err != nil {
			t.Errorf("%s : error received but not expected : %s", e.name, err.Error())
		}

		req.Body.Close()
	}

}

func TestTools_WriteJSONFile(t *testing.T) {
	var testTools Tools

	rr := httptest.NewRecorder()

	payload := JSONResponse{
		Error:   false,
		Message: "foo",
	}

	headers := make(http.Header)
	headers.Add("FOO", "BAR")

	err := testTools.WriteJSON(rr, http.StatusOK, payload, headers)
	if err != nil {
		t.Errorf("failed to write json file : %v", err)
	}
}

func TestTools_Error(t *testing.T) {
	var testTools Tools

	rr := httptest.NewRecorder()

	err := testTools.ErrorJSON(rr, errors.New("some error"), http.StatusServiceUnavailable)
	if err != nil {
		t.Error(err)
	}

	var payload JSONResponse
	decoder := json.NewDecoder(rr.Body)
	err = decoder.Decode(&payload)

	if err != nil {
		t.Error("received error decoding json")
	}

	if !payload.Error {
		t.Error("payload error should be true")
	}

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("wrong status code : expected %d got %d", http.StatusServiceUnavailable, rr.Code)
	}

}
