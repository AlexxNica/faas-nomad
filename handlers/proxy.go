package handlers

import (
	"bytes"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/hashicorp/faas-nomad/consul"
	"github.com/hashicorp/faas-nomad/nomad"
)

// MakeProxy creates a proxy for HTTP web requests which can be routed to a function.
func MakeProxy(client nomad.Job, resolver *consul.ConsulResolver) http.HandlerFunc {
	proxyClient := http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   3 * time.Second,
				KeepAlive: 0,
			}).DialContext,
			MaxIdleConns:          1,
			DisableKeepAlives:     true,
			IdleConnTimeout:       120 * time.Millisecond,
			ExpectContinueTimeout: 1500 * time.Millisecond,
		},
	}

	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		if r.Method == "POST" {

			vars := mux.Vars(r)
			service := vars["name"]

			stamp := strconv.FormatInt(time.Now().Unix(), 10)

			defer func(when time.Time) {
				seconds := time.Since(when).Seconds()
				log.Printf("[%s] took %f seconds\n", stamp, seconds)
			}(time.Now())

			requestBody, _ := ioutil.ReadAll(r.Body)
			defer r.Body.Close()

			urls, _ := resolver.Resolve(service)

			// hack for docker for mac, need real implementation
			address := urls[0]
			if strings.Contains(address, "127.0.0.1") {
				address = strings.Replace(address, "127.0.0.1", "docker.for.mac.localhost", 1)
			}

			log.Println("Trying to call:", address)
			request, _ := http.NewRequest("POST", address, bytes.NewReader(requestBody))

			copyHeaders(&request.Header, &r.Header)

			defer request.Body.Close()

			response, err := proxyClient.Do(request)
			if err != nil {
				log.Println(err.Error())
				writeHead(service, http.StatusInternalServerError, w)
				buf := bytes.NewBufferString("Can't reach service: " + service)
				w.Write(buf.Bytes())
				return
			}

			clientHeader := w.Header()
			copyHeaders(&clientHeader, &response.Header)

			// TODO: copyHeaders removes the need for this line - test removal.
			// Match header for strict services
			w.Header().Set("Content-Type", r.Header.Get("Content-Type"))

			responseBody, _ := ioutil.ReadAll(response.Body)

			writeHead(service, http.StatusOK, w)
			w.Write(responseBody)

		}
	}
}

func writeHead(service string, code int, w http.ResponseWriter) {
	w.WriteHeader(code)
}

func copyHeaders(destination *http.Header, source *http.Header) {
	for k, vv := range *source {
		vvClone := make([]string, len(vv))
		copy(vvClone, vv)
		(*destination)[k] = vvClone
	}
}

func randomInt(min, max int) int {
	rand.Seed(time.Now().Unix())
	return rand.Intn(max-min) + min
}
