package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/launchdarkly/go-server-sdk.v5/testservice/servicedef"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	ld "gopkg.in/launchdarkly/go-server-sdk.v5"

	"github.com/gorilla/mux"
)

const clientsBasePath = "/clients/"
const clientPath = clientsBasePath + "{id}"

var capabilities = []string{
	servicedef.CapabilityServerSide,
	servicedef.CapabilityStronglyTyped,
	servicedef.CapabilityAllFlagsClientSideOnly,
	servicedef.CapabilityAllFlagsDetailsOnlyForTrackedFlags,
	servicedef.CapabilityAllFlagsWithReasons,
	servicedef.CapabilityBigSegments,
	servicedef.CapabilityServerSidePolling,
	servicedef.CapabilityServiceEndpoints,
	servicedef.CapabilityTags,
}

// gets the specified environment variable, or the default if not set
func getenv(envVar, defaultVal string) string {
	ret := os.Getenv(envVar)
	if len(ret) == 0 {
		return defaultVal
	}
	return ret
}

type TestService struct {
	name          string
	Handler       http.Handler
	clients       map[string]*SDKClientEntity
	clientCounter int
	loggers       ldlog.Loggers
	lock          sync.Mutex
}

type HTTPStatusError interface {
	HTTPStatus() int
}

type BadRequestError struct {
	Message string
}

func (e BadRequestError) Error() string {
	return e.Message
}

func (e BadRequestError) HTTPStatus() int {
	return http.StatusBadRequest
}

type NotFoundError struct{}

func (e NotFoundError) Error() string {
	return "not found"
}

func (e NotFoundError) HTTPStatus() int {
	return http.StatusNotFound
}

func NewTestService(loggers ldlog.Loggers, name string) *TestService {
	service := &TestService{
		name:    name,
		clients: make(map[string]*SDKClientEntity),
		loggers: loggers,
	}

	router := mux.NewRouter()

	router.HandleFunc("/", service.GetStatus).Methods("GET")
	router.HandleFunc("/", service.DeleteStopService).Methods("DELETE")
	router.HandleFunc("/", service.PostCreateClient).Methods("POST")
	router.HandleFunc(clientPath, service.DeleteClient).Methods("DELETE")
	router.HandleFunc(clientPath, service.PostCommand).Methods("POST")

	service.Handler = router
	return service
}

func (s *TestService) GetStatus(w http.ResponseWriter, r *http.Request) {
	rep := servicedef.StatusRep{
		Name:          s.name,
		Capabilities:  capabilities,
		ClientVersion: ld.Version,
	}
	writeJSON(w, rep)
}

func (s *TestService) DeleteStopService(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Test service has told us to exit")
	os.Exit(0)
}

func (s *TestService) PostCreateClient(w http.ResponseWriter, r *http.Request) {
	var p servicedef.CreateInstanceParams
	if err := readJSON(r, &p); err != nil {
		writeError(w, err)
		return
	}

	loggers := s.loggers
	loggers.SetPrefix(fmt.Sprintf("[sdklog:%s] ", p.Tag))

	loggers.Info("Creating ")
	c, err := NewSDKClientEntity(p)
	if err != nil {
		writeError(w, err)
		return
	}

	s.lock.Lock()
	s.clientCounter++
	id := strconv.Itoa(s.clientCounter)
	s.clients[id] = c
	s.lock.Unlock()

	url := clientsBasePath + id
	w.Header().Set("Location", url)
	w.WriteHeader(http.StatusCreated)
}

func (s *TestService) DeleteClient(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	s.lock.Lock()
	c := s.clients[id]
	if c != nil {
		delete(s.clients, id)
	}
	s.lock.Unlock()

	if c == nil {
		writeError(w, NotFoundError{})
		return
	}

	c.Close()

	w.WriteHeader(http.StatusNoContent)
}

func (s *TestService) PostCommand(w http.ResponseWriter, r *http.Request) {
	c, _, err := s.getClient(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var p servicedef.CommandParams
	if err := readJSON(r, &p); err != nil {
		writeError(w, err)
		return
	}
	result, err := c.DoCommand(p)
	if err != nil {
		writeError(w, err)
		return
	}
	if result == nil {
		w.WriteHeader(http.StatusAccepted)
	} else {
		writeJSON(w, result)
	}
}

func (s *TestService) getClient(r *http.Request) (*SDKClientEntity, string, error) {
	id := mux.Vars(r)["id"]
	s.lock.Lock()
	c := s.clients[id]
	s.lock.Unlock()
	if c != nil {
		return c, id, nil
	}
	return nil, "", NotFoundError{}
}

func readJSON(r *http.Request, dest interface{}) error {
	if r.Body == nil {
		return errors.New("request has no body")
	}
	return json.NewDecoder(r.Body).Decode(dest)
}

func writeJSON(w http.ResponseWriter, rep interface{}) {
	data, _ := json.Marshal(rep)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_, _ = w.Write(data)
}

func writeError(w http.ResponseWriter, err error) {
	status := 500
	if hse, ok := err.(HTTPStatusError); ok {
		status = hse.HTTPStatus()
	}
	w.WriteHeader(status)
	_, _ = w.Write([]byte(err.Error()))
	log.Printf("*** error: %s", err)
}

func logLevelFromName(name string) ldlog.LogLevel {
	switch strings.ToLower(name) {
	case "debug":
		return ldlog.Debug
	case "info":
		return ldlog.Info
	case "warn":
		return ldlog.Warn
	case "error":
		return ldlog.Error
	}
	return ldlog.Debug
}

func main() {
	loggers := ldlog.NewDefaultLoggers()
	loggers.SetMinLevel(logLevelFromName(os.Getenv("LD_LOG_LEVEL")))

	port := "8000"
	service := NewTestService(loggers, "go-server-sdk")
	server := &http.Server{Handler: service.Handler, Addr: ":" + port}
	fmt.Printf("Listening on port %s\n", port)
	err := server.ListenAndServe()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
