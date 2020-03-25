package main

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/containers/buildah/pkg/formats"
	"github.com/containers/libpod/cmd/podman/cliconfig"
	"github.com/containers/libpod/cmd/podman/shared"
	"github.com/containers/libpod/libpod/define"
	"github.com/containers/libpod/pkg/adapter"
	"github.com/docker/go-units"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

const (
	STOPPED      = "Stopped" //nolint
	RUNNING      = "Running"
	PAUSED       = "Paused"
	EXITED       = "Exited"
	ERROR        = "Error"
	CREATED      = "Created"
	NUM_CTR_INFO = 10
)

var (
	bc_opts shared.PsOptions
)

type podPsCtrInfo struct {
	Name   string `json:"name,omitempty"`
	Id     string `json:"id,omitempty"`
	Status string `json:"status,omitempty"`
}

type podPsOptions struct {
	NoTrunc            bool
	Format             string
	Sort               string
	Quiet              bool
	NumberOfContainers bool
	Cgroup             bool
	NamesOfContainers  bool
	IdsOfContainers    bool
	StatusOfContainers bool
}

type podPsTemplateParams struct {
	Created            string
	ID                 string
	Name               string
	NumberOfContainers int
	Status             string
	Cgroup             string
	ContainerInfo      string
	InfraID            string
	Namespaces         string
}

// podPsJSONParams is used as a base structure for the psParams
// If template output is requested, podPsJSONParams will be converted to
// podPsTemplateParams.
// podPsJSONParams will be populated by data from libpod.Container,
// the members of the struct are the sama data types as their sources.
type podPsJSONParams struct {
	CreatedAt          time.Time      `json:"createdAt"`
	ID                 string         `json:"id"`
	Name               string         `json:"name"`
	NumberOfContainers int            `json:"numberOfContainers"`
	Status             string         `json:"status"`
	CtrsInfo           []podPsCtrInfo `json:"containerInfo,omitempty"`
	Cgroup             string         `json:"cgroup,omitempty"`
	InfraID            string         `json:"infraContainerId,omitempty"`
	Namespaces         []string       `json:"namespaces,omitempty"`
}

// Type declaration and functions for sorting the pod PS output
type podPsSorted []podPsJSONParams

func (a podPsSorted) Len() int      { return len(a) }
func (a podPsSorted) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

type podPsSortedCreated struct{ podPsSorted }

func (a podPsSortedCreated) Less(i, j int) bool {
	return a.podPsSorted[i].CreatedAt.After(a.podPsSorted[j].CreatedAt)
}

type podPsSortedId struct{ podPsSorted }

func (a podPsSortedId) Less(i, j int) bool { return a.podPsSorted[i].ID < a.podPsSorted[j].ID }

type podPsSortedNumber struct{ podPsSorted }

func (a podPsSortedNumber) Less(i, j int) bool {
	return len(a.podPsSorted[i].CtrsInfo) < len(a.podPsSorted[j].CtrsInfo)
}

type podPsSortedName struct{ podPsSorted }

func (a podPsSortedName) Less(i, j int) bool { return a.podPsSorted[i].Name < a.podPsSorted[j].Name }

type podPsSortedStatus struct{ podPsSorted }

func (a podPsSortedStatus) Less(i, j int) bool {
	return a.podPsSorted[i].Status < a.podPsSorted[j].Status
}

var (
	podPsCommand cliconfig.PodPsValues

	podPsDescription = "List all pods on system including their names, ids and current state."
	_podPsCommand    = &cobra.Command{
		Use:     "ps",
		Aliases: []string{"ls", "list"},
		Args:    noSubArgs,
		Short:   "List pods",
		Long:    podPsDescription,
		RunE: func(cmd *cobra.Command, args []string) error {
			podPsCommand.InputArgs = args
			podPsCommand.GlobalFlags = MainGlobalOpts
			podPsCommand.Remote = remoteclient
			return podPsCmd(&podPsCommand)
		},
	}
)

func init() {
	podPsCommand.Command = _podPsCommand
	podPsCommand.SetHelpTemplate(HelpTemplate())
	podPsCommand.SetUsageTemplate(UsageTemplate())
	flags := podPsCommand.Flags()
	flags.BoolVar(&podPsCommand.CtrNames, "ctr-names", false, "Display the container names")
	flags.BoolVar(&podPsCommand.CtrIDs, "ctr-ids", false, "Display the container UUIDs. If no-trunc is not set they will be truncated")
	flags.BoolVar(&podPsCommand.CtrStatus, "ctr-status", false, "Display the container status")
	flags.StringVarP(&podPsCommand.Filter, "filter", "f", "", "Filter output based on conditions given")
	flags.StringVar(&podPsCommand.Format, "format", "", "Pretty-print pods to JSON or using a Go template")
	flags.BoolVarP(&podPsCommand.Latest, "latest", "l", false, "Act on the latest pod podman is aware of")
	flags.BoolVar(&podPsCommand.Namespace, "namespace", false, "Display namespace information of the pod")
	flags.BoolVar(&podPsCommand.Namespace, "ns", false, "Display namespace information of the pod")
	flags.BoolVar(&podPsCommand.NoTrunc, "no-trunc", false, "Do not truncate pod and container IDs")
	flags.BoolVarP(&podPsCommand.Quiet, "quiet", "q", false, "Print the numeric IDs of the pods only")
	flags.StringVar(&podPsCommand.Sort, "sort", "created", "Sort output by created, id, name, or number")
	markFlagHiddenForRemoteClient("latest", flags)
}

func podPsCmd(c *cliconfig.PodPsValues) error {
	if err := podPsCheckFlagsPassed(c); err != nil {
		return errors.Wrapf(err, "error with flags passed")
	}

	runtime, err := adapter.GetRuntime(getContext(), &c.PodmanCommand)
	if err != nil {
		return errors.Wrapf(err, "error creating libpod runtime")
	}
	defer runtime.DeferredShutdown(false)

	opts := podPsOptions{
		NoTrunc:            c.NoTrunc,
		Quiet:              c.Quiet,
		Sort:               c.Sort,
		IdsOfContainers:    c.CtrIDs,
		NamesOfContainers:  c.CtrNames,
		StatusOfContainers: c.CtrStatus,
	}

	opts.Format = genPodPsFormat(c)

	var pods []*adapter.Pod

	// If latest is set true filters are ignored.
	if c.Latest {
		pod, err := runtime.GetLatestPod()
		if err != nil {
			return err
		}
		pods = append(pods, pod)
		return generatePodPsOutput(pods, opts)
	}

	if c.Filter != "" {
		pods, err = runtime.GetPodsWithFilters(c.Filter)
		if err != nil {
			return err
		}
	} else {
		pods, err = runtime.GetAllPods()
		if err != nil {
			return err
		}
	}

	return generatePodPsOutput(pods, opts)
}

// podPsCheckFlagsPassed checks if mutually exclusive flags are passed together
func podPsCheckFlagsPassed(c *cliconfig.PodPsValues) error {
	// quiet, and format with Go template are mutually exclusive
	flags := 0
	if c.Quiet {
		flags++
	}
	if c.Flag("format").Changed && c.Format != formats.JSONString {
		flags++
	}
	if flags > 1 {
		return errors.Errorf("quiet and format with Go template are mutually exclusive")
	}
	return nil
}

// generate the template based on conditions given
func genPodPsFormat(c *cliconfig.PodPsValues) string {
	format := ""
	switch {
	case c.Format != "":
		// "\t" from the command line is not being recognized as a tab
		// replacing the string "\t" to a tab character if the user passes in "\t"
		format = strings.Replace(c.Format, `\t`, "\t", -1)
	case c.Quiet:
		format = formats.IDString
	default:
		format = "table {{.ID}}\t{{.Name}}\t{{.Status}}\t{{.Created}}"
		if c.Bool("namespace") {
			format += "\t{{.Cgroup}}\t{{.Namespaces}}"
		}
		if c.CtrNames || c.CtrIDs || c.CtrStatus {
			format += "\t{{.ContainerInfo}}"
		} else {
			format += "\t{{.NumberOfContainers}}"
		}
		format += "\t{{.InfraID}}"
	}
	return format
}

func podPsToGeneric(templParams []podPsTemplateParams, jsonParams []podPsJSONParams) (genericParams []interface{}) {
	if len(templParams) > 0 {
		for _, v := range templParams {
			genericParams = append(genericParams, interface{}(v))
		}
		return
	}
	for _, v := range jsonParams {
		genericParams = append(genericParams, interface{}(v))
	}
	return
}

// generate the accurate header based on template given
func (p *podPsTemplateParams) podHeaderMap() map[string]string {
	v := reflect.Indirect(reflect.ValueOf(p))
	values := make(map[string]string)

	for i := 0; i < v.NumField(); i++ {
		key := v.Type().Field(i).Name
		value := key
		if value == "ID" {
			value = "Pod" + value
		}
		if value == "NumberOfContainers" {
			value = "#OfContainers"
		}
		values[key] = strings.ToUpper(splitCamelCase(value))
	}
	return values
}

func sortPodPsOutput(sortBy string, psOutput podPsSorted) (podPsSorted, error) {
	switch sortBy {
	case "created":
		sort.Sort(podPsSortedCreated{psOutput})
	case "id":
		sort.Sort(podPsSortedId{psOutput})
	case "name":
		sort.Sort(podPsSortedName{psOutput})
	case "number":
		sort.Sort(podPsSortedNumber{psOutput})
	case "status":
		sort.Sort(podPsSortedStatus{psOutput})
	default:
		return nil, errors.Errorf("invalid option for --sort, options are: id, names, or number")
	}
	return psOutput, nil
}

// getPodTemplateOutput returns the modified container information
func getPodTemplateOutput(psParams []podPsJSONParams, opts podPsOptions) ([]podPsTemplateParams, error) {
	var (
		psOutput []podPsTemplateParams
	)

	for _, psParam := range psParams {
		podID := psParam.ID
		infraID := psParam.InfraID
		var ctrStr string

		truncated := ""
		if !opts.NoTrunc {
			podID = shortID(podID)
			if len(psParam.CtrsInfo) > NUM_CTR_INFO {
				psParam.CtrsInfo = psParam.CtrsInfo[:NUM_CTR_INFO]
				truncated = "..."
			}
			infraID = shortID(infraID)
		}
		for _, ctrInfo := range psParam.CtrsInfo {
			infoSlice := make([]string, 0)
			if opts.IdsOfContainers {
				if opts.NoTrunc {
					infoSlice = append(infoSlice, ctrInfo.Id)
				} else {
					infoSlice = append(infoSlice, shortID(ctrInfo.Id))
				}
			}
			if opts.NamesOfContainers {
				infoSlice = append(infoSlice, ctrInfo.Name)
			}
			if opts.StatusOfContainers {
				infoSlice = append(infoSlice, ctrInfo.Status)
			}
			if len(infoSlice) != 0 {
				ctrStr += fmt.Sprintf("[%s] ", strings.Join(infoSlice, ","))
			}
		}
		ctrStr += truncated
		params := podPsTemplateParams{
			Created:            units.HumanDuration(time.Since(psParam.CreatedAt)) + " ago",
			ID:                 podID,
			Name:               psParam.Name,
			Status:             psParam.Status,
			NumberOfContainers: psParam.NumberOfContainers,
			Cgroup:             psParam.Cgroup,
			ContainerInfo:      ctrStr,
			InfraID:            infraID,
			Namespaces:         strings.Join(psParam.Namespaces, ","),
		}

		psOutput = append(psOutput, params)
	}

	return psOutput, nil
}

func getNamespaces(pod *adapter.Pod) []string {
	var shared []string
	if pod.SharesPID() {
		shared = append(shared, "pid")
	}
	if pod.SharesNet() {
		shared = append(shared, "net")
	}
	if pod.SharesMount() {
		shared = append(shared, "mnt")
	}
	if pod.SharesIPC() {
		shared = append(shared, "ipc")
	}
	if pod.SharesUser() {
		shared = append(shared, "user")
	}
	if pod.SharesCgroup() {
		shared = append(shared, "cgroup")
	}
	if pod.SharesUTS() {
		shared = append(shared, "uts")
	}
	return shared
}

// getAndSortPodJSONOutput returns the container info in its raw, sorted form
func getAndSortPodJSONParams(pods []*adapter.Pod, opts podPsOptions) ([]podPsJSONParams, error) {
	var (
		psOutput []podPsJSONParams
	)

	for _, pod := range pods {
		ctrs, err := pod.AllContainers()
		ctrsInfo := make([]podPsCtrInfo, 0)
		if err != nil {
			return nil, err
		}
		ctrNum := len(ctrs)
		status, err := pod.GetPodStatus()
		if err != nil {
			return nil, err
		}

		infraID, err := pod.InfraContainerID()
		if err != nil {
			return nil, err
		}
		for _, ctr := range ctrs {
			batchInfo, err := adapter.BatchContainerOp(ctr, bc_opts)
			if err != nil {
				return nil, err
			}
			var status string
			switch batchInfo.ConState {
			case define.ContainerStateExited:
				fallthrough
			case define.ContainerStateStopped:
				status = EXITED
			case define.ContainerStateRunning:
				status = RUNNING
			case define.ContainerStatePaused:
				status = PAUSED
			case define.ContainerStateCreated, define.ContainerStateConfigured:
				status = CREATED
			default:
				status = ERROR
			}
			ctrsInfo = append(ctrsInfo, podPsCtrInfo{
				Name:   batchInfo.ConConfig.Name,
				Id:     ctr.ID(),
				Status: status,
			})
		}
		params := podPsJSONParams{
			CreatedAt:          pod.CreatedTime(),
			ID:                 pod.ID(),
			Name:               pod.Name(),
			Status:             status,
			Cgroup:             pod.CgroupParent(),
			NumberOfContainers: ctrNum,
			CtrsInfo:           ctrsInfo,
			Namespaces:         getNamespaces(pod),
			InfraID:            infraID,
		}

		psOutput = append(psOutput, params)
	}
	return sortPodPsOutput(opts.Sort, psOutput)
}

func generatePodPsOutput(pods []*adapter.Pod, opts podPsOptions) error {
	if len(pods) == 0 && opts.Format != formats.JSONString {
		return nil
	}
	psOutput, err := getAndSortPodJSONParams(pods, opts)
	if err != nil {
		return err
	}
	var out formats.Writer

	switch opts.Format {
	case formats.JSONString:
		out = formats.JSONStructArray{Output: podPsToGeneric([]podPsTemplateParams{}, psOutput)}
	default:
		psOutput, err := getPodTemplateOutput(psOutput, opts)
		if err != nil {
			return errors.Wrapf(err, "unable to create output")
		}
		out = formats.StdoutTemplateArray{Output: podPsToGeneric(psOutput, []podPsJSONParams{}), Template: opts.Format, Fields: psOutput[0].podHeaderMap()}
	}

	return out.Out()
}
