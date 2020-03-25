package compat

import (
	"encoding/binary"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/containers/libpod/libpod"
	"github.com/containers/libpod/libpod/define"
	"github.com/containers/libpod/libpod/logs"
	"github.com/containers/libpod/pkg/api/handlers"
	"github.com/containers/libpod/pkg/api/handlers/utils"
	"github.com/containers/libpod/pkg/signal"
	"github.com/containers/libpod/pkg/util"
	"github.com/gorilla/schema"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

func RemoveContainer(w http.ResponseWriter, r *http.Request) {
	decoder := r.Context().Value("decoder").(*schema.Decoder)
	query := struct {
		Force bool `schema:"force"`
		Vols  bool `schema:"v"`
		Link  bool `schema:"link"`
	}{
		// override any golang type defaults
	}

	if err := decoder.Decode(&query, r.URL.Query()); err != nil {
		utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
			errors.Wrapf(err, "Failed to parse parameters for %s", r.URL.String()))
		return
	}

	if query.Link && !utils.IsLibpodRequest(r) {
		utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
			utils.ErrLinkNotSupport)
		return
	}

	runtime := r.Context().Value("runtime").(*libpod.Runtime)
	name := utils.GetName(r)
	con, err := runtime.LookupContainer(name)
	if err != nil {
		utils.ContainerNotFound(w, name, err)
		return
	}

	if err := runtime.RemoveContainer(r.Context(), con, query.Force, query.Vols); err != nil {
		utils.InternalServerError(w, err)
		return
	}
	utils.WriteResponse(w, http.StatusNoContent, "")
}

func ListContainers(w http.ResponseWriter, r *http.Request) {
	var (
		containers []*libpod.Container
		err        error
	)
	runtime := r.Context().Value("runtime").(*libpod.Runtime)
	decoder := r.Context().Value("decoder").(*schema.Decoder)
	query := struct {
		All     bool                `schema:"all"`
		Limit   int                 `schema:"limit"`
		Size    bool                `schema:"size"`
		Filters map[string][]string `schema:"filters"`
	}{
		// override any golang type defaults
	}

	if err := decoder.Decode(&query, r.URL.Query()); err != nil {
		utils.Error(w, "Something went wrong.", http.StatusBadRequest, errors.Wrapf(err, "Failed to parse parameters for %s", r.URL.String()))
		return
	}
	if query.All {
		containers, err = runtime.GetAllContainers()
	} else {
		containers, err = runtime.GetRunningContainers()
	}
	if err != nil {
		utils.InternalServerError(w, err)
		return
	}
	if _, found := r.URL.Query()["limit"]; found && query.Limit != -1 {
		last := query.Limit
		if len(containers) > last {
			containers = containers[len(containers)-last:]
		}
	}
	// TODO filters still need to be applied
	infoData, err := runtime.Info()
	if err != nil {
		utils.InternalServerError(w, errors.Wrapf(err, "Failed to obtain system info"))
		return
	}

	var list = make([]*handlers.Container, len(containers))
	for i, ctnr := range containers {
		api, err := handlers.LibpodToContainer(ctnr, infoData, query.Size)
		if err != nil {
			utils.InternalServerError(w, err)
			return
		}
		list[i] = api
	}
	utils.WriteResponse(w, http.StatusOK, list)
}

func GetContainer(w http.ResponseWriter, r *http.Request) {
	runtime := r.Context().Value("runtime").(*libpod.Runtime)
	decoder := r.Context().Value("decoder").(*schema.Decoder)
	query := struct {
		Size bool `schema:"size"`
	}{
		// override any golang type defaults
	}

	if err := decoder.Decode(&query, r.URL.Query()); err != nil {
		utils.Error(w, "Something went wrong.", http.StatusBadRequest, errors.Wrapf(err, "Failed to parse parameters for %s", r.URL.String()))
		return
	}

	name := utils.GetName(r)
	ctnr, err := runtime.LookupContainer(name)
	if err != nil {
		utils.ContainerNotFound(w, name, err)
		return
	}
	api, err := handlers.LibpodToContainerJSON(ctnr, query.Size)
	if err != nil {
		utils.InternalServerError(w, err)
		return
	}
	utils.WriteResponse(w, http.StatusOK, api)
}

func KillContainer(w http.ResponseWriter, r *http.Request) {
	// /{version}/containers/(name)/kill
	runtime := r.Context().Value("runtime").(*libpod.Runtime)
	decoder := r.Context().Value("decoder").(*schema.Decoder)
	query := struct {
		Signal string `schema:"signal"`
	}{
		Signal: "KILL",
	}
	if err := decoder.Decode(&query, r.URL.Query()); err != nil {
		utils.Error(w, "Something went wrong.", http.StatusBadRequest, errors.Wrapf(err, "Failed to parse parameters for %s", r.URL.String()))
		return
	}

	sig, err := signal.ParseSignalNameOrNumber(query.Signal)
	if err != nil {
		utils.InternalServerError(w, err)
		return
	}
	name := utils.GetName(r)
	con, err := runtime.LookupContainer(name)
	if err != nil {
		utils.ContainerNotFound(w, name, err)
		return
	}

	state, err := con.State()
	if err != nil {
		utils.InternalServerError(w, err)
		return
	}

	// If the Container is stopped already, send a 409
	if state == define.ContainerStateStopped || state == define.ContainerStateExited {
		utils.Error(w, fmt.Sprintf("Container %s is not running", name), http.StatusConflict, errors.New(fmt.Sprintf("Cannot kill Container %s, it is not running", name)))
		return
	}

	err = con.Kill(uint(sig))
	if err != nil {
		utils.Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrapf(err, "unable to kill Container %s", name))
	}

	if utils.IsLibpodRequest(r) {
		// the kill behavior for docker differs from podman in that they appear to wait
		// for the Container to croak so the exit code is accurate immediately after the
		// kill is sent.  libpod does not.  but we can add a wait here only for the docker
		// side of things and mimic that behavior
		if _, err = con.Wait(); err != nil {
			utils.Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrapf(err, "failed to wait for Container %s", con.ID()))
			return
		}
	}
	// Success
	utils.WriteResponse(w, http.StatusNoContent, "")
}

func WaitContainer(w http.ResponseWriter, r *http.Request) {
	var msg string
	// /{version}/containers/(name)/wait
	exitCode, err := utils.WaitContainer(w, r)
	if err != nil {
		return
	}
	utils.WriteResponse(w, http.StatusOK, handlers.ContainerWaitOKBody{
		StatusCode: int(exitCode),
		Error: struct {
			Message string
		}{
			Message: msg,
		},
	})
}

func LogsFromContainer(w http.ResponseWriter, r *http.Request) {
	decoder := r.Context().Value("decoder").(*schema.Decoder)
	runtime := r.Context().Value("runtime").(*libpod.Runtime)

	query := struct {
		Follow     bool   `schema:"follow"`
		Stdout     bool   `schema:"stdout"`
		Stderr     bool   `schema:"stderr"`
		Since      string `schema:"since"`
		Until      string `schema:"until"`
		Timestamps bool   `schema:"timestamps"`
		Tail       string `schema:"tail"`
	}{
		Tail: "all",
	}
	if err := decoder.Decode(&query, r.URL.Query()); err != nil {
		utils.Error(w, "Something went wrong.", http.StatusBadRequest, errors.Wrapf(err, "Failed to parse parameters for %s", r.URL.String()))
		return
	}

	if !(query.Stdout || query.Stderr) {
		msg := fmt.Sprintf("%s: you must choose at least one stream", http.StatusText(http.StatusBadRequest))
		utils.Error(w, msg, http.StatusBadRequest, errors.Errorf("%s for %s", msg, r.URL.String()))
		return
	}

	name := utils.GetName(r)
	ctnr, err := runtime.LookupContainer(name)
	if err != nil {
		utils.ContainerNotFound(w, name, err)
		return
	}

	var tail int64 = -1
	if query.Tail != "all" {
		tail, err = strconv.ParseInt(query.Tail, 0, 64)
		if err != nil {
			utils.BadRequest(w, "tail", query.Tail, err)
			return
		}
	}

	var since time.Time
	if _, found := r.URL.Query()["since"]; found {
		since, err = util.ParseInputTime(query.Since)
		if err != nil {
			utils.BadRequest(w, "since", query.Since, err)
			return
		}
	}

	var until time.Time
	if _, found := r.URL.Query()["until"]; found {
		since, err = util.ParseInputTime(query.Until)
		if err != nil {
			utils.BadRequest(w, "until", query.Until, err)
			return
		}
	}

	options := &logs.LogOptions{
		Details:    true,
		Follow:     query.Follow,
		Since:      since,
		Tail:       tail,
		Timestamps: query.Timestamps,
	}

	var wg sync.WaitGroup
	options.WaitGroup = &wg

	logChannel := make(chan *logs.LogLine, tail+1)
	if err := runtime.Log([]*libpod.Container{ctnr}, options, logChannel); err != nil {
		utils.InternalServerError(w, errors.Wrapf(err, "Failed to obtain logs for Container '%s'", name))
		return
	}
	go func() {
		wg.Wait()
		close(logChannel)
	}()

	w.WriteHeader(http.StatusOK)
	var builder strings.Builder
	for ok := true; ok; ok = query.Follow {
		for line := range logChannel {
			if _, found := r.URL.Query()["until"]; found {
				if line.Time.After(until) {
					break
				}
			}

			// Reset variables we're ready to loop again
			builder.Reset()
			header := [8]byte{}

			switch line.Device {
			case "stdout":
				if !query.Stdout {
					continue
				}
				header[0] = 1
			case "stderr":
				if !query.Stderr {
					continue
				}
				header[0] = 2
			default:
				// Logging and moving on is the best we can do here. We may have already sent
				// a Status and Content-Type to client therefore we can no longer report an error.
				log.Infof("unknown Device type '%s' in log file from Container %s", line.Device, ctnr.ID())
				continue
			}

			if query.Timestamps {
				builder.WriteString(line.Time.Format(time.RFC3339))
				builder.WriteRune(' ')
			}
			builder.WriteString(line.Msg)
			// Build header and output entry
			binary.BigEndian.PutUint32(header[4:], uint32(len(header)+builder.Len()))
			if _, err := w.Write(header[:]); err != nil {
				log.Errorf("unable to write log output header: %q", err)
			}
			if _, err := fmt.Fprint(w, builder.String()); err != nil {
				log.Errorf("unable to write builder string: %q", err)
			}
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}
}
