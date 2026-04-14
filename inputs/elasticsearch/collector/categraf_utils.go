// external categraf used for elasticsearch

package collector

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"k8s.io/klog/v2"
)

func GetNodeID(client *http.Client, user, password, s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL %s: %s", s, err)
	}
	if user != "" && password != "" {
		u.User = url.UserPassword(user, password)
	}
	var nsr nodeStatsResponse
	u.Path = path.Join(u.Path, "/_nodes/_local/name")
	res, err := client.Get(u.String())
	if err != nil {
		return "", fmt.Errorf("failed to get node ID from %s: %s", u.String(), err)
	}
	defer func() {
		err = res.Body.Close()
		if err != nil {
			klog.ErrorS(err, "failed to close elasticsearch response body")
		}
	}()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP Request failed with code %d", res.StatusCode)
	}

	bts, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(bts, &nsr); err != nil {
		return "", err
	}

	// Only 1 should be returned
	for id := range nsr.Nodes {
		return id, nil
	}
	return "", nil
}

func GetClusterName(client *http.Client, user, password, s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL %s: %s", s, err)
	}
	if user != "" && password != "" {
		u.User = url.UserPassword(user, password)
	}

	var cir ClusterInfoResponse
	res, err := client.Get(u.String())
	if err != nil {
		return "", fmt.Errorf("failed to get cluster info from %s: %s", u.String(), err)
	}
	defer func() {
		err = res.Body.Close()
		if err != nil {
			klog.ErrorS(err, "failed to close elasticsearch response body")
		}
	}()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP Request failed with code %d", res.StatusCode)
	}
	bts, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(bts, &cir); err != nil {
		return "", err
	}
	return cir.ClusterName, nil
}

func GetCatMaster(client *http.Client, user, password, s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL %s: %s", s, err)
	}
	if user != "" && password != "" {
		u.User = url.UserPassword(user, password)
	}
	u.Path = path.Join(u.Path, "/_cat/master")
	res, err := client.Get(u.String())
	if err != nil {
		return "", fmt.Errorf("failed to get node ID from %s: %s", u.String(), err)
	}
	defer func() {
		err = res.Body.Close()
		if err != nil {
			klog.ErrorS(err, "failed to close elasticsearch response body")
		}
	}()

	if res.StatusCode != http.StatusOK {
		// NOTE: we are not going to read/discard r.Body under the assumption we'd prefer
		// to let the underlying transport close the connection and re-establish a new one for
		// future calls.
		return "", fmt.Errorf("elasticsearch: Unable to retrieve master node information. API responded with status-code %d, expected %d", res.StatusCode, http.StatusOK)
	}
	response, err := io.ReadAll(res.Body)

	if err != nil {
		return "", err
	}

	masterID := strings.Split(string(response), " ")[0]

	return masterID, nil
}

func queryURL(client *http.Client, u *url.URL) ([]byte, error) {
	res, err := client.Get(u.String())
	if err != nil {
		return []byte{}, fmt.Errorf("failed to get resource from %s://%s:%s%s: %s",
			u.Scheme, u.Hostname(), u.Port(), u.Path, err)
	}

	defer func() {
		err = res.Body.Close()
		if err != nil {
			klog.ErrorS(err, "failed to close elasticsearch response body")
		}
	}()

	if res.StatusCode != http.StatusOK {
		return []byte{}, fmt.Errorf("HTTP Request failed with code %d", res.StatusCode)
	}

	bts, err := io.ReadAll(res.Body)
	if err != nil {
		return []byte{}, err
	}

	return bts, nil
}
