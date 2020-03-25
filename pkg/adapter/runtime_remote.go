// +build remoteclient

package adapter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/containers/buildah/imagebuildah"
	"github.com/containers/buildah/pkg/formats"
	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/types"
	"github.com/containers/libpod/cmd/podman/cliconfig"
	"github.com/containers/libpod/cmd/podman/remoteclientconfig"
	iopodman "github.com/containers/libpod/cmd/podman/varlink"
	"github.com/containers/libpod/libpod"
	"github.com/containers/libpod/libpod/define"
	"github.com/containers/libpod/libpod/events"
	"github.com/containers/libpod/libpod/image"
	"github.com/containers/libpod/pkg/util"
	"github.com/containers/libpod/utils"
	"github.com/containers/storage/pkg/archive"
	"github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/varlink/go/varlink"
	v1 "k8s.io/api/core/v1"
)

// ImageRuntime is wrapper for image runtime
type RemoteImageRuntime struct{}

// RemoteRuntime describes a wrapper runtime struct
type RemoteRuntime struct {
	Conn   *varlink.Connection
	Remote bool
	cmd    cliconfig.MainFlags
	config io.Reader
}

// LocalRuntime describes a typical libpod runtime
type LocalRuntime struct {
	*RemoteRuntime
}

// GetRuntimeNoStore returns a LocalRuntime struct with the actual runtime embedded in it
// The nostore is ignored
func GetRuntimeNoStore(ctx context.Context, c *cliconfig.PodmanCommand) (*LocalRuntime, error) {
	return GetRuntime(ctx, c)
}

// GetRuntime returns a LocalRuntime struct with the actual runtime embedded in it
func GetRuntime(ctx context.Context, c *cliconfig.PodmanCommand) (*LocalRuntime, error) {
	var (
		customConfig bool
		err          error
		f            *os.File
	)
	runtime := RemoteRuntime{
		Remote: true,
		cmd:    c.GlobalFlags,
	}
	configPath := remoteclientconfig.GetConfigFilePath()
	// Check if the basedir for configPath exists and if not, create it.
	if _, err := os.Stat(filepath.Dir(configPath)); os.IsNotExist(err) {
		if mkdirErr := os.MkdirAll(filepath.Dir(configPath), 0750); mkdirErr != nil {
			return nil, mkdirErr
		}
	}
	if len(c.GlobalFlags.RemoteConfigFilePath) > 0 {
		configPath = c.GlobalFlags.RemoteConfigFilePath
		customConfig = true
	}

	f, err = os.Open(configPath)
	if err != nil {
		// If user does not explicitly provide a configuration file path and we cannot
		// find a default, no error should occur.
		if os.IsNotExist(err) && !customConfig {
			logrus.Debugf("unable to load configuration file at %s", configPath)
			runtime.config = nil
		} else {
			return nil, errors.Wrapf(err, "unable to load configuration file at %s", configPath)
		}
	} else {
		// create the io reader for the remote client
		runtime.config = bufio.NewReader(f)
	}
	conn, err := runtime.Connect()
	if err != nil {
		return nil, err
	}
	runtime.Conn = conn
	return &LocalRuntime{
		&runtime,
	}, nil
}

// DeferredShutdown is a bogus wrapper for compaat with the libpod
// runtime and should only be run when a defer is being used
func (r RemoteRuntime) DeferredShutdown(force bool) {
	if err := r.Shutdown(force); err != nil {
		logrus.Error("unable to shutdown runtime")
	}
}

// RuntimeConfig is a bogus wrapper for compat with the libpod runtime
type RuntimeConfig struct {
	// CGroupManager is the CGroup Manager to use
	// Valid values are "cgroupfs" and "systemd"
	CgroupManager string
}

// Shutdown is a bogus wrapper for compat with the libpod runtime
func (r *RemoteRuntime) GetConfig() (*RuntimeConfig, error) {
	return nil, nil
}

// Shutdown is a bogus wrapper for compat with the libpod runtime
func (r RemoteRuntime) Shutdown(force bool) error {
	return nil
}

// ContainerImage
type ContainerImage struct {
	remoteImage
}

type remoteImage struct {
	ID           string
	Labels       map[string]string
	RepoTags     []string
	RepoDigests  []string
	Parent       string
	Size         int64
	Created      time.Time
	InputName    string
	Names        []string
	Digest       digest.Digest
	Digests      []digest.Digest
	isParent     bool
	Runtime      *LocalRuntime
	TopLayer     string
	ReadOnly     bool
	NamesHistory []string
}

// Container ...
type Container struct {
	remoteContainer
}

// remoteContainer ....
type remoteContainer struct {
	Runtime *LocalRuntime
	config  *libpod.ContainerConfig
	state   *libpod.ContainerState
}

// Pod ...
type Pod struct {
	remotepod
}

type remotepod struct {
	config     *libpod.PodConfig
	state      *libpod.PodInspectState
	containers []libpod.PodContainerInfo
	Runtime    *LocalRuntime
}

type VolumeFilter func(*Volume) bool

// Volume is embed for libpod volumes
type Volume struct {
	remoteVolume
}

type remoteVolume struct {
	Runtime *LocalRuntime
	config  *libpod.VolumeConfig
}

// GetImages returns a slice of containerimages over a varlink connection
func (r *LocalRuntime) GetImages() ([]*ContainerImage, error) {
	return r.getImages(false)
}

// GetRWImages returns a slice of read/write containerimages over a varlink connection
func (r *LocalRuntime) GetRWImages() ([]*ContainerImage, error) {
	return r.getImages(true)
}

func (r *LocalRuntime) GetFilteredImages(filters []string, rwOnly bool) ([]*ContainerImage, error) {
	if len(filters) > 0 {
		return nil, errors.Wrap(define.ErrNotImplemented, "filtering images is not supported on the remote client")
	}
	var newImages []*ContainerImage
	images, err := iopodman.ListImages().Call(r.Conn)
	if err != nil {
		return nil, err
	}
	for _, i := range images {
		if rwOnly && i.ReadOnly {
			continue
		}
		name := i.Id
		if len(i.RepoTags) > 1 {
			name = i.RepoTags[0]
		}
		newImage, err := imageInListToContainerImage(i, name, r)
		if err != nil {
			return nil, err
		}
		newImages = append(newImages, newImage)
	}
	return newImages, nil
}
func (r *LocalRuntime) getImages(rwOnly bool) ([]*ContainerImage, error) {
	var newImages []*ContainerImage
	images, err := iopodman.ListImages().Call(r.Conn)
	if err != nil {
		return nil, err
	}
	for _, i := range images {
		if rwOnly && i.ReadOnly {
			continue
		}
		name := i.Id
		if len(i.RepoTags) > 1 {
			name = i.RepoTags[0]
		}
		newImage, err := imageInListToContainerImage(i, name, r)
		if err != nil {
			return nil, err
		}
		newImages = append(newImages, newImage)
	}
	return newImages, nil
}

func imageInListToContainerImage(i iopodman.Image, name string, runtime *LocalRuntime) (*ContainerImage, error) {
	created, err := time.ParseInLocation(time.RFC3339, i.Created, time.UTC)
	if err != nil {
		return nil, err
	}
	var digests []digest.Digest
	for _, d := range i.Digests {
		digests = append(digests, digest.Digest(d))
	}
	ri := remoteImage{
		InputName:    name,
		ID:           i.Id,
		Digest:       digest.Digest(i.Digest),
		Digests:      digests,
		Labels:       i.Labels,
		RepoTags:     i.RepoTags,
		RepoDigests:  i.RepoTags,
		Parent:       i.ParentId,
		Size:         i.Size,
		Created:      created,
		Names:        i.RepoTags,
		isParent:     i.IsParent,
		Runtime:      runtime,
		TopLayer:     i.TopLayer,
		ReadOnly:     i.ReadOnly,
		NamesHistory: i.History,
	}
	return &ContainerImage{ri}, nil
}

// NewImageFromLocal returns a container image representation of a image over varlink
func (r *LocalRuntime) NewImageFromLocal(name string) (*ContainerImage, error) {
	img, err := iopodman.GetImage().Call(r.Conn, name)
	if err != nil {
		return nil, err
	}
	return imageInListToContainerImage(img, name, r)

}

// LoadFromArchiveReference creates an image from a local archive
func (r *LocalRuntime) LoadFromArchiveReference(ctx context.Context, srcRef types.ImageReference, signaturePolicyPath string, writer io.Writer) ([]*ContainerImage, error) {
	var iid string
	creds := iopodman.AuthConfig{}
	reply, err := iopodman.PullImage().Send(r.Conn, varlink.More, srcRef.DockerReference().String(), creds)
	if err != nil {
		return nil, err
	}

	for {
		responses, flags, err := reply()
		if err != nil {
			return nil, err
		}
		for _, line := range responses.Logs {
			fmt.Print(line)
		}
		iid = responses.Id
		if flags&varlink.Continues == 0 {
			break
		}
	}

	newImage, err := r.NewImageFromLocal(iid)
	if err != nil {
		return nil, err
	}
	return []*ContainerImage{newImage}, nil
}

// New calls into local storage to look for an image in local storage or to pull it
func (r *LocalRuntime) New(ctx context.Context, name, signaturePolicyPath, authfile string, writer io.Writer, dockeroptions *image.DockerRegistryOptions, signingoptions image.SigningOptions, label *string, pullType util.PullType) (*ContainerImage, error) {
	var iid string
	if label != nil {
		return nil, errors.New("the remote client function does not support checking a remote image for a label")
	}
	creds := iopodman.AuthConfig{}
	if dockeroptions.DockerRegistryCreds != nil {
		creds.Username = dockeroptions.DockerRegistryCreds.Username
		creds.Password = dockeroptions.DockerRegistryCreds.Password
	}
	reply, err := iopodman.PullImage().Send(r.Conn, varlink.More, name, creds)
	if err != nil {
		return nil, err
	}
	for {
		responses, flags, err := reply()
		if err != nil {
			return nil, err
		}
		for _, line := range responses.Logs {
			fmt.Print(line)
		}
		iid = responses.Id
		if flags&varlink.Continues == 0 {
			break
		}
	}
	newImage, err := r.NewImageFromLocal(iid)
	if err != nil {
		return nil, err
	}
	return newImage, nil
}

func (r *LocalRuntime) ImageTree(imageOrID string, whatRequires bool) (string, error) {
	return iopodman.ImageTree().Call(r.Conn, imageOrID, whatRequires)
}

// IsParent goes through the layers in the store and checks if i.TopLayer is
// the parent of any other layer in store. Double check that image with that
// layer exists as well.
func (ci *ContainerImage) IsParent(context.Context) (bool, error) {
	return ci.remoteImage.isParent, nil
}

// ID returns the image ID as a string
func (ci *ContainerImage) ID() string {
	return ci.remoteImage.ID
}

// Names returns a string array of names associated with the image
func (ci *ContainerImage) Names() []string {
	return ci.remoteImage.Names
}

// NamesHistory returns a string array of names previously associated with the image
func (ci *ContainerImage) NamesHistory() []string {
	return ci.remoteImage.NamesHistory
}

// Created returns the time the image was created
func (ci *ContainerImage) Created() time.Time {
	return ci.remoteImage.Created
}

// IsReadOnly returns whether the image is ReadOnly
func (ci *ContainerImage) IsReadOnly() bool {
	return ci.remoteImage.ReadOnly
}

// Size returns the size of the image
func (ci *ContainerImage) Size(ctx context.Context) (*uint64, error) {
	usize := uint64(ci.remoteImage.Size)
	return &usize, nil
}

// Digest returns the image's digest
func (ci *ContainerImage) Digest() digest.Digest {
	return ci.remoteImage.Digest
}

// Digests returns the image's digests
func (ci *ContainerImage) Digests() []digest.Digest {
	return append([]digest.Digest{}, ci.remoteImage.Digests...)
}

// Labels returns a map of the image's labels
func (ci *ContainerImage) Labels(ctx context.Context) (map[string]string, error) {
	return ci.remoteImage.Labels, nil
}

// Dangling returns a bool if the image is "dangling"
func (ci *ContainerImage) Dangling() bool {
	return len(ci.Names()) == 0
}

// TopLayer returns an images top layer as a string
func (ci *ContainerImage) TopLayer() string {
	return ci.remoteImage.TopLayer
}

// TagImage ...
func (ci *ContainerImage) TagImage(tag string) error {
	_, err := iopodman.TagImage().Call(ci.Runtime.Conn, ci.ID(), tag)
	return err
}

// UntagImage removes a single tag from an image
func (ci *ContainerImage) UntagImage(tag string) error {
	_, err := iopodman.UntagImage().Call(ci.Runtime.Conn, ci.ID(), tag)
	return err
}

// RemoveImage calls varlink to remove an image
func (r *LocalRuntime) RemoveImage(ctx context.Context, img *ContainerImage, force bool) (*image.ImageDeleteResponse, error) {
	ir := image.ImageDeleteResponse{}
	response, err := iopodman.RemoveImageWithResponse().Call(r.Conn, img.InputName, force)
	if err != nil {
		return nil, err
	}
	ir.Deleted = response.Deleted
	ir.Untagged = append(ir.Untagged, response.Untagged...)
	return &ir, nil
}

// History returns the history of an image and its layers
func (ci *ContainerImage) History(ctx context.Context) ([]*image.History, error) {
	var imageHistories []*image.History

	reply, err := iopodman.HistoryImage().Call(ci.Runtime.Conn, ci.InputName)
	if err != nil {
		return nil, err
	}
	for _, h := range reply {
		created, err := time.ParseInLocation(time.RFC3339, h.Created, time.UTC)
		if err != nil {
			return nil, err
		}
		ih := image.History{
			ID:        h.Id,
			Created:   &created,
			CreatedBy: h.CreatedBy,
			Size:      h.Size,
			Comment:   h.Comment,
		}
		imageHistories = append(imageHistories, &ih)
	}
	return imageHistories, nil
}

// PruneImages is the wrapper call for a remote-client to prune images
func (r *LocalRuntime) PruneImages(ctx context.Context, all bool, filter []string) ([]string, error) {
	return iopodman.ImagesPrune().Call(r.Conn, all, filter)
}

// Export is a wrapper to container export to a tarfile
func (r *LocalRuntime) Export(name string, path string) error {
	tempPath, err := iopodman.ExportContainer().Call(r.Conn, name, "")
	if err != nil {
		return err
	}
	return r.GetFileFromRemoteHost(tempPath, path, true)
}

func (r *LocalRuntime) GetFileFromRemoteHost(remoteFilePath, outputPath string, delete bool) error {
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer outputFile.Close()

	writer := bufio.NewWriter(outputFile)
	defer writer.Flush()

	reply, err := iopodman.ReceiveFile().Send(r.Conn, varlink.Upgrade, remoteFilePath, delete)
	if err != nil {
		return err
	}

	length, _, err := reply()
	if err != nil {
		return errors.Wrap(err, "unable to get file length for transfer")
	}

	reader := r.Conn.Reader
	if _, err := io.CopyN(writer, reader, length); err != nil {
		return errors.Wrap(err, "file transfer failed")
	}
	return nil
}

// Import implements the remote calls required to import a container image to the store
func (r *LocalRuntime) Import(ctx context.Context, source, reference string, changes []string, history string, quiet bool) (string, error) {
	// First we send the file to the host
	tempFile, err := r.SendFileOverVarlink(source)
	if err != nil {
		return "", err
	}
	return iopodman.ImportImage().Call(r.Conn, strings.TrimRight(tempFile, ":"), reference, history, changes, true)
}

func (r *LocalRuntime) Build(ctx context.Context, c *cliconfig.BuildValues, options imagebuildah.BuildOptions, dockerfiles []string) (string, reference.Canonical, error) {
	buildOptions := iopodman.BuildOptions{
		AddHosts:     options.CommonBuildOpts.AddHost,
		CgroupParent: options.CommonBuildOpts.CgroupParent,
		CpuPeriod:    int64(options.CommonBuildOpts.CPUPeriod),
		CpuQuota:     options.CommonBuildOpts.CPUQuota,
		CpuShares:    int64(options.CommonBuildOpts.CPUShares),
		CpusetCpus:   options.CommonBuildOpts.CPUSetMems,
		CpusetMems:   options.CommonBuildOpts.CPUSetMems,
		Memory:       options.CommonBuildOpts.Memory,
		MemorySwap:   options.CommonBuildOpts.MemorySwap,
		ShmSize:      options.CommonBuildOpts.ShmSize,
		Ulimit:       options.CommonBuildOpts.Ulimit,
		Volume:       options.CommonBuildOpts.Volumes,
	}

	buildinfo := iopodman.BuildInfo{
		AdditionalTags:        options.AdditionalTags,
		Annotations:           options.Annotations,
		BuildArgs:             options.Args,
		BuildOptions:          buildOptions,
		CniConfigDir:          options.CNIConfigDir,
		CniPluginDir:          options.CNIPluginPath,
		Compression:           string(options.Compression),
		DefaultsMountFilePath: options.DefaultMountsFilePath,
		Dockerfiles:           dockerfiles,
		// Err: string(options.Err),
		ForceRmIntermediateCtrs: options.ForceRmIntermediateCtrs,
		Iidfile:                 options.IIDFile,
		Label:                   options.Labels,
		Layers:                  options.Layers,
		Nocache:                 options.NoCache,
		// Out:
		Output:                 options.Output,
		OutputFormat:           options.OutputFormat,
		PullPolicy:             options.PullPolicy.String(),
		Quiet:                  options.Quiet,
		RemoteIntermediateCtrs: options.RemoveIntermediateCtrs,
		// ReportWriter:
		RuntimeArgs: options.RuntimeArgs,
		Squash:      options.Squash,
	}
	// tar the file
	outputFile, err := ioutil.TempFile("", "varlink_tar_send")
	if err != nil {
		return "", nil, err
	}
	defer outputFile.Close()
	defer os.Remove(outputFile.Name())

	// Create the tarball of the context dir to a tempfile
	if err := utils.TarToFilesystem(options.ContextDirectory, outputFile); err != nil {
		return "", nil, err
	}
	// Send the context dir tarball over varlink.
	tempFile, err := r.SendFileOverVarlink(outputFile.Name())
	if err != nil {
		return "", nil, err
	}
	buildinfo.ContextDir = tempFile

	reply, err := iopodman.BuildImage().Send(r.Conn, varlink.More, buildinfo)
	if err != nil {
		return "", nil, err
	}

	for {
		responses, flags, err := reply()
		if err != nil {
			return "", nil, err
		}
		for _, line := range responses.Logs {
			fmt.Print(line)
		}
		if flags&varlink.Continues == 0 {
			break
		}
	}
	return "", nil, err
}

// SendFileOverVarlink sends a file over varlink in an upgraded connection
func (r *LocalRuntime) SendFileOverVarlink(source string) (string, error) {
	fs, err := os.Open(source)
	if err != nil {
		return "", err
	}

	fileInfo, err := fs.Stat()
	if err != nil {
		return "", err
	}
	logrus.Debugf("sending %s over varlink connection", source)
	reply, err := iopodman.SendFile().Send(r.Conn, varlink.Upgrade, "", int64(fileInfo.Size()))
	if err != nil {
		return "", err
	}
	_, _, err = reply()
	if err != nil {
		return "", err
	}

	reader := bufio.NewReader(fs)
	_, err = reader.WriteTo(r.Conn.Writer)
	if err != nil {
		return "", err
	}
	logrus.Debugf("file transfer complete for %s", source)
	r.Conn.Writer.Flush()

	// All was sent, wait for the ACK from the server
	tempFile, err := r.Conn.Reader.ReadString(':')
	if err != nil {
		return "", err
	}

	// r.Conn is kaput at this point due to the upgrade
	if err := r.RemoteRuntime.RefreshConnection(); err != nil {
		return "", err

	}

	return strings.Replace(tempFile, ":", "", -1), nil
}

// GetAllVolumes retrieves all the volumes
func (r *LocalRuntime) GetAllVolumes() ([]*libpod.Volume, error) {
	return nil, define.ErrNotImplemented
}

// RemoveVolume removes a volumes
func (r *LocalRuntime) RemoveVolume(ctx context.Context, v *libpod.Volume, force, prune bool) error {
	return define.ErrNotImplemented
}

// GetContainers retrieves all containers from the state
// Filters can be provided which will determine what containers are included in
// the output. Multiple filters are handled by ANDing their output, so only
// containers matching all filters are returned
func (r *LocalRuntime) GetContainers(filters ...libpod.ContainerFilter) ([]*libpod.Container, error) {
	return nil, define.ErrNotImplemented
}

// RemoveContainer removes the given container
// If force is specified, the container will be stopped first
// Otherwise, RemoveContainer will return an error if the container is running
func (r *LocalRuntime) RemoveContainer(ctx context.Context, c *libpod.Container, force, volumes bool) error {
	return define.ErrNotImplemented
}

// CreateVolume creates a volume over a varlink connection for the remote client
func (r *LocalRuntime) CreateVolume(ctx context.Context, c *cliconfig.VolumeCreateValues, labels, opts map[string]string) (string, error) {
	cvOpts := iopodman.VolumeCreateOpts{
		Options: opts,
		Labels:  labels,
	}
	if len(c.InputArgs) > 0 {
		cvOpts.VolumeName = c.InputArgs[0]
	}

	if c.Flag("driver").Changed {
		cvOpts.Driver = c.Driver
	}

	return iopodman.VolumeCreate().Call(r.Conn, cvOpts)
}

// RemoveVolumes removes volumes over a varlink connection for the remote client
func (r *LocalRuntime) RemoveVolumes(ctx context.Context, c *cliconfig.VolumeRmValues) ([]string, map[string]error, error) {
	rmOpts := iopodman.VolumeRemoveOpts{
		All:     c.All,
		Force:   c.Force,
		Volumes: c.InputArgs,
	}
	success, failures, err := iopodman.VolumeRemove().Call(r.Conn, rmOpts)
	stringsToErrors := make(map[string]error)
	for k, v := range failures {
		stringsToErrors[k] = errors.New(v)
	}
	return success, stringsToErrors, err
}

func (r *LocalRuntime) Push(ctx context.Context, srcName, destination, manifestMIMEType, authfile, digestfile, signaturePolicyPath string, writer io.Writer, forceCompress bool, signingOptions image.SigningOptions, dockerRegistryOptions *image.DockerRegistryOptions, additionalDockerArchiveTags []reference.NamedTagged) error {

	reply, err := iopodman.PushImage().Send(r.Conn, varlink.More, srcName, destination, forceCompress, manifestMIMEType, signingOptions.RemoveSignatures, signingOptions.SignBy)
	if err != nil {
		return err
	}
	for {
		responses, flags, err := reply()
		if err != nil {
			return err
		}
		for _, line := range responses.Logs {
			fmt.Print(line)
		}
		if flags&varlink.Continues == 0 {
			break
		}
	}

	return err
}

// InspectVolumes returns a slice of volumes based on an arg list or --all
func (r *LocalRuntime) InspectVolumes(ctx context.Context, c *cliconfig.VolumeInspectValues) ([]*libpod.InspectVolumeData, error) {
	var (
		inspectData []*libpod.InspectVolumeData
		volumes     []string
	)

	if c.All {
		allVolumes, err := r.Volumes(ctx)
		if err != nil {
			return nil, err
		}
		for _, vol := range allVolumes {
			volumes = append(volumes, vol.Name())
		}
	} else {
		for _, arg := range c.InputArgs {
			volumes = append(volumes, arg)
		}
	}

	for _, vol := range volumes {
		jsonString, err := iopodman.InspectVolume().Call(r.Conn, vol)
		if err != nil {
			return nil, err
		}
		inspectJSON := new(libpod.InspectVolumeData)
		if err := json.Unmarshal([]byte(jsonString), inspectJSON); err != nil {
			return nil, errors.Wrapf(err, "error unmarshalling inspect JSON for volume %s", vol)
		}
		inspectData = append(inspectData, inspectJSON)
	}

	return inspectData, nil
}

// Volumes returns a slice of adapter.volumes based on information about libpod
// volumes over a varlink connection
func (r *LocalRuntime) Volumes(ctx context.Context) ([]*Volume, error) {
	reply, err := iopodman.GetVolumes().Call(r.Conn, []string{}, true)
	if err != nil {
		return nil, err
	}
	return varlinkVolumeToVolume(r, reply), nil
}

func varlinkVolumeToVolume(r *LocalRuntime, volumes []iopodman.Volume) []*Volume {
	var vols []*Volume
	for _, v := range volumes {
		volumeConfig := libpod.VolumeConfig{
			Name:       v.Name,
			Labels:     v.Labels,
			MountPoint: v.MountPoint,
			Driver:     v.Driver,
			Options:    v.Options,
		}
		n := remoteVolume{
			Runtime: r,
			config:  &volumeConfig,
		}
		newVol := Volume{
			n,
		}
		vols = append(vols, &newVol)
	}
	return vols
}

// PruneVolumes removes all unused volumes from the remote system
func (r *LocalRuntime) PruneVolumes(ctx context.Context) ([]string, []error) {
	var errs []error
	prunedNames, prunedErrors, err := iopodman.VolumesPrune().Call(r.Conn)
	if err != nil {
		return []string{}, []error{err}
	}
	// We need to transform the string results of the error into actual error types
	for _, e := range prunedErrors {
		errs = append(errs, errors.New(e))
	}
	return prunedNames, errs
}

// SaveImage is a wrapper function for saving an image to the local filesystem
func (r *LocalRuntime) SaveImage(ctx context.Context, c *cliconfig.SaveValues) error {
	source := c.InputArgs[0]
	additionalTags := c.InputArgs[1:]

	options := iopodman.ImageSaveOptions{
		Name:     source,
		Format:   c.Format,
		Output:   c.Output,
		MoreTags: additionalTags,
		Quiet:    c.Quiet,
		Compress: c.Compress,
	}
	reply, err := iopodman.ImageSave().Send(r.Conn, varlink.More, options)
	if err != nil {
		return err
	}

	var fetchfile string
	for {
		responses, flags, err := reply()
		if err != nil {
			return err
		}
		if len(responses.Id) > 0 {
			fetchfile = responses.Id
		}
		for _, line := range responses.Logs {
			fmt.Print(line)
		}
		if flags&varlink.Continues == 0 {
			break
		}

	}
	if err != nil {
		return err
	}

	outputToDir := false
	outfile := c.Output
	var outputFile *os.File
	// If the result is supposed to be a dir, then we need to put the tarfile
	// from the host in a temporary file
	if options.Format != "oci-archive" && options.Format != "docker-archive" {
		outputToDir = true
		outputFile, err = ioutil.TempFile("", "saveimage_tempfile")
		if err != nil {
			return err
		}
		outfile = outputFile.Name()
		defer outputFile.Close()
		defer os.Remove(outputFile.Name())
	}
	// We now need to fetch the tarball result back to the more system
	if err := r.GetFileFromRemoteHost(fetchfile, outfile, true); err != nil {
		return err
	}

	// If the result is a tarball, we're done
	// If it is a dir, we need to untar the temporary file into the dir
	if outputToDir {
		if err := utils.UntarToFileSystem(c.Output, outputFile, &archive.TarOptions{}); err != nil {
			return err
		}
	}
	return nil
}

// LoadImage loads a container image from a remote client's filesystem
func (r *LocalRuntime) LoadImage(ctx context.Context, name string, cli *cliconfig.LoadValues) (string, error) {
	var names string
	remoteTempFile, err := r.SendFileOverVarlink(cli.Input)
	if err != nil {
		return "", nil
	}
	more := varlink.More
	if cli.Quiet {
		more = 0
	}
	reply, err := iopodman.LoadImage().Send(r.Conn, uint64(more), name, remoteTempFile, cli.Quiet, true)
	if err != nil {
		return "", err
	}

	for {
		responses, flags, err := reply()
		if err != nil {
			logrus.Error(err)
			return "", err
		}
		for _, line := range responses.Logs {
			fmt.Print(line)
		}
		names = responses.Id
		if flags&varlink.Continues == 0 {
			break
		}
	}
	return names, nil
}

// IsImageNotFound checks if the error indicates that no image was found.
func IsImageNotFound(err error) bool {
	if errors.Cause(err) == image.ErrNoSuchImage {
		return true
	}
	switch err.(type) {
	case *iopodman.ImageNotFound:
		return true
	}
	return false
}

// HealthCheck executes a container's healthcheck over a varlink connection
func (r *LocalRuntime) HealthCheck(c *cliconfig.HealthCheckValues) (string, error) {
	return iopodman.HealthCheckRun().Call(r.Conn, c.InputArgs[0])
}

// Events monitors libpod/podman events over a varlink connection
func (r *LocalRuntime) Events(c *cliconfig.EventValues) error {
	var more uint64
	if c.Stream {
		more = uint64(varlink.More)
	}
	reply, err := iopodman.GetEvents().Send(r.Conn, more, c.Filter, c.Since, c.Until)
	if err != nil {
		return errors.Wrapf(err, "unable to obtain events")
	}

	w := bufio.NewWriter(os.Stdout)
	var tmpl *template.Template
	if c.Format != formats.JSONString {
		template, err := template.New("events").Parse(c.Format)
		if err != nil {
			return err
		}
		tmpl = template
	}

	for {
		returnedEvent, flags, err := reply()
		if err != nil {
			// When the error handling is back into podman, we can flip this to a better way to check
			// for problems. For now, this works.
			return err
		}
		if returnedEvent.Time == "" && returnedEvent.Status == "" && returnedEvent.Type == "" {
			// We got a blank event return, signals end of stream in certain cases
			break
		}
		eTime, err := time.Parse(time.RFC3339Nano, returnedEvent.Time)
		if err != nil {
			return errors.Wrapf(err, "unable to parse time of event %s", returnedEvent.Time)
		}
		eType, err := events.StringToType(returnedEvent.Type)
		if err != nil {
			return err
		}
		eStatus, err := events.StringToStatus(returnedEvent.Status)
		if err != nil {
			return err
		}
		event := events.Event{
			ID:     returnedEvent.Id,
			Image:  returnedEvent.Image,
			Name:   returnedEvent.Name,
			Status: eStatus,
			Time:   eTime,
			Type:   eType,
		}
		if c.Format == formats.JSONString {
			jsonStr, err := event.ToJSONString()
			if err != nil {
				return errors.Wrapf(err, "unable to format json")
			}
			if _, err := w.Write([]byte(jsonStr)); err != nil {
				return err
			}
		} else if len(c.Format) > 0 {
			if err := tmpl.Execute(w, event); err != nil {
				return err
			}
		} else {
			if _, err := w.Write([]byte(event.ToHumanReadable())); err != nil {
				return err
			}
		}
		if _, err := w.Write([]byte("\n")); err != nil {
			return err
		}
		if err := w.Flush(); err != nil {
			return err
		}
		if flags&varlink.Continues == 0 {
			break
		}
	}
	return nil
}

// Diff ...
func (r *LocalRuntime) Diff(c *cliconfig.DiffValues, to string) ([]archive.Change, error) {
	var changes []archive.Change
	reply, err := iopodman.Diff().Call(r.Conn, to)
	if err != nil {
		return nil, err
	}
	for _, change := range reply {
		changes = append(changes, archive.Change{Path: change.Path, Kind: stringToChangeType(change.ChangeType)})
	}
	return changes, nil
}

func stringToChangeType(change string) archive.ChangeType {
	switch change {
	case "A":
		return archive.ChangeAdd
	case "D":
		return archive.ChangeDelete
	default:
		logrus.Errorf("'%s' is unknown archive type", change)
		fallthrough
	case "C":
		return archive.ChangeModify
	}
}

// GenerateKube creates kubernetes email from containers and pods
func (r *LocalRuntime) GenerateKube(c *cliconfig.GenerateKubeValues) (*v1.Pod, *v1.Service, error) {
	var (
		pod     v1.Pod
		service v1.Service
	)
	reply, err := iopodman.GenerateKube().Call(r.Conn, c.InputArgs[0], c.Service)
	if err != nil {
		return nil, nil, errors.Wrap(err, "unable to create kubernetes YAML")
	}
	if err := json.Unmarshal([]byte(reply.Pod), &pod); err != nil {
		return nil, nil, err
	}
	err = json.Unmarshal([]byte(reply.Service), &service)
	return &pod, &service, err
}

// GetContainersByContext looks up containers based on the cli input of all, latest, or a list
func (r *LocalRuntime) GetContainersByContext(all bool, latest bool, namesOrIDs []string) ([]*Container, error) {
	var containers []*Container
	cids, err := iopodman.GetContainersByContext().Call(r.Conn, all, latest, namesOrIDs)
	if err != nil {
		return nil, err
	}
	for _, cid := range cids {
		ctr, err := r.LookupContainer(cid)
		if err != nil {
			return nil, err
		}
		containers = append(containers, ctr)
	}
	return containers, nil
}

// GetVersion returns version information from service
func (r *LocalRuntime) GetVersion() (define.Version, error) {
	version, goVersion, gitCommit, built, osArch, apiVersion, err := iopodman.GetVersion().Call(r.Conn)
	if err != nil {
		return define.Version{}, errors.Wrapf(err, "Unable to obtain server version information")
	}

	var buildTime int64
	if built != "" {
		t, err := time.Parse(time.RFC3339, built)
		if err != nil {
			return define.Version{}, nil
		}
		buildTime = t.Unix()
	}

	return define.Version{
		RemoteAPIVersion: apiVersion,
		Version:          version,
		GoVersion:        goVersion,
		GitCommit:        gitCommit,
		Built:            buildTime,
		OsArch:           osArch,
	}, nil
}
