package gojenkins

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type Auth struct {
	Username string
	ApiToken string
}

type Jenkins struct {
	auth    *Auth
	baseUrl string
}

func NewJenkins(auth *Auth, baseUrl string) *Jenkins {
	return &Jenkins{
		auth:    auth,
		baseUrl: baseUrl,
	}
}

func (jenkins *Jenkins) buildUrl(path string, params url.Values) (requestUrl string) {
	requestUrl = jenkins.baseUrl + path + "/api/json"
	if params != nil {
		queryString := params.Encode()
		if queryString != "" {
			requestUrl = requestUrl + "?" + queryString
		}
	}

	return
}

func (jenkins *Jenkins) sendRequest(req *http.Request) (*http.Response, error) {
	req.SetBasicAuth(jenkins.auth.Username, jenkins.auth.ApiToken)
	return http.DefaultClient.Do(req)
}

func (jenkins *Jenkins) parseXmlResponse(resp *http.Response, body interface{}) (err error) {
	defer resp.Body.Close()

	if body == nil {
		return
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	return xml.Unmarshal(data, body)
}

func (jenkins *Jenkins) parseResponse(resp *http.Response, body interface{}) (err error) {
	defer resp.Body.Close()

	if body == nil {
		// If the response contains only a location header pointing to a queue item, return that
		// queue item.
		switch body.(type) {
		case *Item:
			loc := resp.Header.Get("Location")
			if loc != "" {
				// FIXME: this will break if jenkins isn't at the root of the webserver url
				itemNo, _ := strconv.Atoi(strings.Split(loc, "/")[5])
				body, err = jenkins.GetQueueItem(itemNo)
				return
			}
		}
		return
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	return json.Unmarshal(data, body)
}

func (jenkins *Jenkins) get(path string, params url.Values, body interface{}) (err error) {
	requestUrl := jenkins.buildUrl(path, params)
	req, err := http.NewRequest("GET", requestUrl, nil)
	if err != nil {
		return
	}

	resp, err := jenkins.sendRequest(req)
	if err != nil {
		return
	}
	return jenkins.parseResponse(resp, body)
}

func (jenkins *Jenkins) getXml(path string, params url.Values, body interface{}) (err error) {
	requestUrl := jenkins.buildUrl(path, params)
	req, err := http.NewRequest("GET", requestUrl, nil)
	if err != nil {
		return
	}

	resp, err := jenkins.sendRequest(req)
	if err != nil {
		return
	}
	return jenkins.parseXmlResponse(resp, body)
}

func (jenkins *Jenkins) post(path string, params url.Values, body interface{}) (err error) {
	requestUrl := jenkins.buildUrl(path, params)
	req, err := http.NewRequest("POST", requestUrl, nil)
	if err != nil {
		return
	}

	resp, err := jenkins.sendRequest(req)
	if err != nil {
		return
	}

	return jenkins.parseResponse(resp, body)
}
func (jenkins *Jenkins) postXml(path string, params url.Values, xmlBody io.Reader, body interface{}) (err error) {
	requestUrl := jenkins.baseUrl + path
	if params != nil {
		queryString := params.Encode()
		if queryString != "" {
			requestUrl = requestUrl + "?" + queryString
		}
	}

	req, err := http.NewRequest("POST", requestUrl, xmlBody)
	if err != nil {
		return
	}

	req.Header.Add("Content-Type", "application/xml")
	resp, err := jenkins.sendRequest(req)
	if err != nil {
		return
	}
	if resp.StatusCode != 200 {
		return errors.New(fmt.Sprintf("error: HTTP POST returned status code returned: %d", resp.StatusCode))
	}

	return jenkins.parseXmlResponse(resp, body)
}

// GetJobs returns all jobs you can read.
func (jenkins *Jenkins) GetJobs() ([]Job, error) {
	var payload = struct {
		Jobs []Job `json:"jobs"`
	}{}
	err := jenkins.get("", nil, &payload)
	return payload.Jobs, err
}

// GetJob returns a job which has specified name.
func (jenkins *Jenkins) GetJob(name string) (job Job, err error) {
	err = jenkins.get(fmt.Sprintf("/job/%s", name), nil, &job)
	return
}

//GetJobConfig returns a maven job, has the one used to create Maven job
func (jenkins *Jenkins) GetJobConfig(name string) (job MavenJobItem, err error) {
	err = jenkins.getXml(fmt.Sprintf("/job/%s/config.xml", name), nil, &job)
	return
}

// GetBuild returns a number-th build result of specified job.
func (jenkins *Jenkins) GetBuild(job Job, number int) (build Build, err error) {
	err = jenkins.get(fmt.Sprintf("/job/%s/%d", job.Name, number), nil, &build)
	return
}

// Create a new job
func (jenkins *Jenkins) CreateJob(mavenJobItem MavenJobItem, jobName string) error {
	mavenJobItemXml, _ := xml.Marshal(mavenJobItem)
	reader := bytes.NewReader(mavenJobItemXml)
	params := url.Values{"name": []string{jobName}}

	return jenkins.postXml("/createItem", params, reader, nil)
}

// Add job to view
func (jenkins *Jenkins) AddJobToView(viewName string, job Job) error {
	params := url.Values{"name": []string{job.Name}}
	return jenkins.post(fmt.Sprintf("/view/%s/addJobToView", viewName), params, nil)
}

// Create a new view
func (jenkins *Jenkins) CreateView(listView ListView) error {
	xmlListView, _ := xml.Marshal(listView)
	reader := bytes.NewReader(xmlListView)
	params := url.Values{"name": []string{listView.Name}}

	return jenkins.postXml("/createView", params, reader, nil)
}

// Create a new build for this job.
// Params can be nil.
func (jenkins *Jenkins) Build(job Job, params url.Values) (item Item, err error) {
	if params == nil {
		err = jenkins.post(fmt.Sprintf("/job/%s/build", job.Name), params, &item)
	} else {
		err = jenkins.post(fmt.Sprintf("/job/%s/buildWithParameters", job.Name), params, &item)
	}
	return
}

// Get the console output from a build.
func (jenkins *Jenkins) GetBuildConsoleOutput(build Build) ([]byte, error) {
	requestUrl := fmt.Sprintf("%s/consoleText", build.Url)
	req, err := http.NewRequest("GET", requestUrl, nil)
	if err != nil {
		return nil, err
	}

	res, err := jenkins.sendRequest(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()
	return ioutil.ReadAll(res.Body)
}

// GetQueue returns the current build queue from Jenkins
func (jenkins *Jenkins) GetQueue() (queue Queue, err error) {
	err = jenkins.get(fmt.Sprintf("/queue"), nil, &queue)
	return
}

// GetQueueItem returns a single queue item
func (jenkins *Jenkins) GetQueueItem(itemNo int) (item Item, err error) {
	err = jenkins.get(fmt.Sprintf("/queue/item/%s", itemNo), nil, &item)
	return
}

// GetArtifact return the content of a build artifact
func (jenkins *Jenkins) GetArtifact(build Build, artifact Artifact) ([]byte, error) {
	requestUrl := fmt.Sprintf("%s/artifact/%s", build.Url, artifact.RelativePath)
	req, err := http.NewRequest("GET", requestUrl, nil)
	if err != nil {
		return nil, err
	}

	res, err := jenkins.sendRequest(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()
	return ioutil.ReadAll(res.Body)
}
