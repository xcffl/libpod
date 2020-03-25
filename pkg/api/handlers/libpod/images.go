package libpod

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/containers/buildah"
	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"github.com/containers/libpod/libpod"
	"github.com/containers/libpod/libpod/image"
	image2 "github.com/containers/libpod/libpod/image"
	"github.com/containers/libpod/pkg/api/handlers"
	"github.com/containers/libpod/pkg/api/handlers/utils"
	"github.com/containers/libpod/pkg/domain/entities"
	"github.com/containers/libpod/pkg/util"
	"github.com/gorilla/schema"
	"github.com/pkg/errors"
)

// Commit
// author string
// "container"
// repo string
// tag string
// message
// pause bool
// changes []string

// create

func ImageExists(w http.ResponseWriter, r *http.Request) {
	runtime := r.Context().Value("runtime").(*libpod.Runtime)
	name := utils.GetName(r)

	_, err := runtime.ImageRuntime().NewFromLocal(name)
	if err != nil {
		utils.Error(w, "Something went wrong.", http.StatusNotFound, errors.Wrapf(err, "Failed to find image %s", name))
		return
	}
	utils.WriteResponse(w, http.StatusNoContent, "")
}

func ImageTree(w http.ResponseWriter, r *http.Request) {
	runtime := r.Context().Value("runtime").(*libpod.Runtime)
	name := utils.GetName(r)

	img, err := runtime.ImageRuntime().NewFromLocal(name)
	if err != nil {
		utils.Error(w, "Something went wrong.", http.StatusNotFound, errors.Wrapf(err, "Failed to find image %s", name))
		return
	}

	decoder := r.Context().Value("decoder").(*schema.Decoder)
	query := struct {
		WhatRequires bool `schema:"whatrequires"`
	}{
		WhatRequires: false,
	}
	if err := decoder.Decode(&query, r.URL.Query()); err != nil {
		utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
			errors.Wrapf(err, "failed to parse parameters for %s", r.URL.String()))
		return
	}

	tree, err := img.GenerateTree(query.WhatRequires)
	if err != nil {
		utils.Error(w, "Server error", http.StatusInternalServerError, errors.Wrapf(err, "failed to generate image tree for %s", name))
		return
	}

	utils.WriteResponse(w, http.StatusOK, tree)
}

func GetImage(w http.ResponseWriter, r *http.Request) {
	name := utils.GetName(r)
	newImage, err := utils.GetImage(r, name)
	if err != nil {
		utils.Error(w, "Something went wrong.", http.StatusNotFound, errors.Wrapf(err, "Failed to find image %s", name))
		return
	}
	inspect, err := newImage.Inspect(r.Context())
	if err != nil {
		utils.Error(w, "Server error", http.StatusInternalServerError, errors.Wrapf(err, "failed in inspect image %s", inspect.ID))
		return
	}
	utils.WriteResponse(w, http.StatusOK, inspect)
}

func GetImages(w http.ResponseWriter, r *http.Request) {
	images, err := utils.GetImages(w, r)
	if err != nil {
		utils.Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrap(err, "Failed get images"))
		return
	}
	var summaries = make([]*entities.ImageSummary, len(images))
	for j, img := range images {
		is, err := handlers.ImageToImageSummary(img)
		if err != nil {
			utils.Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrap(err, "Failed transform image summaries"))
			return
		}
		// libpod has additional fields that we need to populate.
		is.Created = img.Created().Unix()
		is.ReadOnly = img.IsReadOnly()
		summaries[j] = is
	}
	utils.WriteResponse(w, http.StatusOK, summaries)
}

func PruneImages(w http.ResponseWriter, r *http.Request) {
	var (
		all bool
		err error
	)
	runtime := r.Context().Value("runtime").(*libpod.Runtime)
	decoder := r.Context().Value("decoder").(*schema.Decoder)
	query := struct {
		Filters map[string][]string `schema:"filters"`
	}{
		// override any golang type defaults
	}

	if err := decoder.Decode(&query, r.URL.Query()); err != nil {
		utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
			errors.Wrapf(err, "Failed to parse parameters for %s", r.URL.String()))
		return
	}

	var libpodFilters = []string{}
	if _, found := r.URL.Query()["filters"]; found {
		dangling := query.Filters["all"]
		if len(dangling) > 0 {
			all, err = strconv.ParseBool(query.Filters["all"][0])
			if err != nil {
				utils.InternalServerError(w, err)
				return
			}
		}
		// dangling is special and not implemented in the libpod side of things
		delete(query.Filters, "dangling")
		for k, v := range query.Filters {
			libpodFilters = append(libpodFilters, fmt.Sprintf("%s=%s", k, v[0]))
		}
	}
	cids, err := runtime.ImageRuntime().PruneImages(r.Context(), all, libpodFilters)
	if err != nil {
		utils.Error(w, "Something went wrong.", http.StatusInternalServerError, err)
		return
	}
	utils.WriteResponse(w, http.StatusOK, cids)
}

func ExportImage(w http.ResponseWriter, r *http.Request) {
	runtime := r.Context().Value("runtime").(*libpod.Runtime)
	decoder := r.Context().Value("decoder").(*schema.Decoder)
	query := struct {
		Compress bool   `schema:"compress"`
		Format   string `schema:"format"`
	}{
		Format: "docker-archive",
	}

	if err := decoder.Decode(&query, r.URL.Query()); err != nil {
		utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
			errors.Wrapf(err, "Failed to parse parameters for %s", r.URL.String()))
		return
	}

	tmpfile, err := ioutil.TempFile("", "api.tar")
	if err != nil {
		utils.Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrap(err, "unable to create tempfile"))
		return
	}
	if err := tmpfile.Close(); err != nil {
		utils.Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrap(err, "unable to close tempfile"))
		return
	}
	name := utils.GetName(r)
	newImage, err := runtime.ImageRuntime().NewFromLocal(name)
	if err != nil {
		utils.ImageNotFound(w, name, err)
		return
	}

	if err := newImage.Save(r.Context(), name, query.Format, tmpfile.Name(), []string{}, false, query.Compress); err != nil {
		utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest, err)
		return
	}
	rdr, err := os.Open(tmpfile.Name())
	if err != nil {
		utils.Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrap(err, "failed to read the exported tarfile"))
		return
	}
	defer rdr.Close()
	defer os.Remove(tmpfile.Name())
	utils.WriteResponse(w, http.StatusOK, rdr)
}

func ImagesLoad(w http.ResponseWriter, r *http.Request) {
	runtime := r.Context().Value("runtime").(*libpod.Runtime)
	decoder := r.Context().Value("decoder").(*schema.Decoder)
	query := struct {
		Reference string `schema:"reference"`
	}{
		// Add defaults here once needed.
	}

	if err := decoder.Decode(&query, r.URL.Query()); err != nil {
		utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
			errors.Wrapf(err, "failed to parse parameters for %s", r.URL.String()))
		return
	}

	tmpfile, err := ioutil.TempFile("", "libpod-images-load.tar")
	if err != nil {
		utils.Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrap(err, "unable to create tempfile"))
		return
	}
	defer os.Remove(tmpfile.Name())
	defer tmpfile.Close()

	if _, err := io.Copy(tmpfile, r.Body); err != nil && err != io.EOF {
		utils.Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrap(err, "unable to write archive to temporary file"))
		return
	}

	tmpfile.Close()
	loadedImage, err := runtime.LoadImage(context.Background(), query.Reference, tmpfile.Name(), os.Stderr, "")
	if err != nil {
		utils.Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrap(err, "unable to load image"))
		return
	}
	split := strings.Split(loadedImage, ",")
	newImage, err := runtime.ImageRuntime().NewFromLocal(split[0])
	if err != nil {
		utils.InternalServerError(w, err)
		return
	}
	// TODO this should go into libpod proper at some point.
	if len(query.Reference) > 0 {
		if err := newImage.TagImage(query.Reference); err != nil {
			utils.InternalServerError(w, err)
			return
		}
	}
	utils.WriteResponse(w, http.StatusOK, handlers.LibpodImagesLoadReport{ID: loadedImage})
}

func ImagesImport(w http.ResponseWriter, r *http.Request) {
	runtime := r.Context().Value("runtime").(*libpod.Runtime)
	decoder := r.Context().Value("decoder").(*schema.Decoder)
	query := struct {
		Changes   []string `schema:"changes"`
		Message   string   `schema:"message"`
		Reference string   `schema:"reference"`
		URL       string   `schema:"URL"`
	}{
		// Add defaults here once needed.
	}

	if err := decoder.Decode(&query, r.URL.Query()); err != nil {
		utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
			errors.Wrapf(err, "failed to parse parameters for %s", r.URL.String()))
		return
	}

	// Check if we need to load the image from a URL or from the request's body.
	source := query.URL
	if len(query.URL) == 0 {
		tmpfile, err := ioutil.TempFile("", "libpod-images-import.tar")
		if err != nil {
			utils.Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrap(err, "unable to create tempfile"))
			return
		}
		defer os.Remove(tmpfile.Name())
		defer tmpfile.Close()

		if _, err := io.Copy(tmpfile, r.Body); err != nil && err != io.EOF {
			utils.Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrap(err, "unable to write archive to temporary file"))
			return
		}

		tmpfile.Close()
		source = tmpfile.Name()
	}
	importedImage, err := runtime.Import(context.Background(), source, query.Reference, query.Changes, query.Message, true)
	if err != nil {
		utils.Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrap(err, "unable to import image"))
		return
	}

	utils.WriteResponse(w, http.StatusOK, handlers.LibpodImagesImportReport{ID: importedImage})
}

func ImagesPull(w http.ResponseWriter, r *http.Request) {
	runtime := r.Context().Value("runtime").(*libpod.Runtime)
	decoder := r.Context().Value("decoder").(*schema.Decoder)
	query := struct {
		Reference    string `schema:"reference"`
		Credentials  string `schema:"credentials"`
		OverrideOS   string `schema:"overrideOS"`
		OverrideArch string `schema:"overrideArch"`
		TLSVerify    bool   `schema:"tlsVerify"`
		AllTags      bool   `schema:"allTags"`
	}{
		TLSVerify: true,
	}

	if err := decoder.Decode(&query, r.URL.Query()); err != nil {
		utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
			errors.Wrapf(err, "failed to parse parameters for %s", r.URL.String()))
		return
	}

	if len(query.Reference) == 0 {
		utils.InternalServerError(w, errors.New("reference parameter cannot be empty"))
		return
	}
	// Enforce the docker transport.  This is just a precaution as some callers
	// might accustomed to using the "transport:reference" notation.  Using
	// another than the "docker://" transport does not really make sense for a
	// remote case. For loading tarballs, the load and import endpoints should
	// be used.
	imageRef, err := alltransports.ParseImageName(query.Reference)
	if err == nil && imageRef.Transport().Name() != docker.Transport.Name() {
		utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
			errors.Errorf("reference %q must be a docker reference", query.Reference))
		return
	} else if err != nil {
		origErr := err
		imageRef, err = alltransports.ParseImageName(fmt.Sprintf("%s://%s", docker.Transport.Name(), query.Reference))
		if err != nil {
			utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
				errors.Wrapf(origErr, "reference %q must be a docker reference", query.Reference))
			return
		}
	}

	// all-tags doesn't work with a tagged reference, so let's check early
	namedRef, err := reference.Parse(query.Reference)
	if err != nil {
		utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
			errors.Wrapf(err, "error parsing reference %q", query.Reference))
		return
	}
	if _, isTagged := namedRef.(reference.Tagged); isTagged && query.AllTags {
		utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
			errors.Errorf("reference %q must not have a tag for all-tags", query.Reference))
		return
	}

	var registryCreds *types.DockerAuthConfig
	if len(query.Credentials) != 0 {
		creds, err := util.ParseRegistryCreds(query.Credentials)
		if err != nil {
			utils.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest,
				errors.Wrapf(err, "error parsing credentials %q", query.Credentials))
			return
		}
		registryCreds = creds
	}

	// Setup the registry options
	dockerRegistryOptions := image.DockerRegistryOptions{
		DockerRegistryCreds: registryCreds,
		OSChoice:            query.OverrideOS,
		ArchitectureChoice:  query.OverrideArch,
	}
	if query.TLSVerify {
		dockerRegistryOptions.DockerInsecureSkipTLSVerify = types.NewOptionalBool(!query.TLSVerify)
	}

	// Prepare the images we want to pull
	imagesToPull := []string{}
	res := []handlers.LibpodImagesPullReport{}
	imageName := namedRef.String()

	if !query.AllTags {
		imagesToPull = append(imagesToPull, imageName)
	} else {
		systemContext := image.GetSystemContext("", "", false)
		tags, err := docker.GetRepositoryTags(context.Background(), systemContext, imageRef)
		if err != nil {
			utils.InternalServerError(w, errors.Wrap(err, "error getting repository tags"))
			return
		}
		for _, tag := range tags {
			imagesToPull = append(imagesToPull, fmt.Sprintf("%s:%s", imageName, tag))
		}
	}

	// Finally pull the images
	for _, img := range imagesToPull {
		newImage, err := runtime.ImageRuntime().New(
			context.Background(),
			img,
			"",
			"",
			os.Stderr,
			&dockerRegistryOptions,
			image.SigningOptions{},
			nil,
			util.PullImageAlways)
		if err != nil {
			utils.InternalServerError(w, errors.Wrapf(err, "error pulling image %q", query.Reference))
			return
		}
		res = append(res, handlers.LibpodImagesPullReport{ID: newImage.ID()})
	}

	utils.WriteResponse(w, http.StatusOK, res)
}

func CommitContainer(w http.ResponseWriter, r *http.Request) {
	var (
		destImage string
		mimeType  string
	)
	decoder := r.Context().Value("decoder").(*schema.Decoder)
	runtime := r.Context().Value("runtime").(*libpod.Runtime)

	query := struct {
		Author    string   `schema:"author"`
		Changes   []string `schema:"changes"`
		Comment   string   `schema:"comment"`
		Container string   `schema:"container"`
		Format    string   `schema:"format"`
		Pause     bool     `schema:"pause"`
		Repo      string   `schema:"repo"`
		Tag       string   `schema:"tag"`
	}{
		Format: "oci",
	}

	if err := decoder.Decode(&query, r.URL.Query()); err != nil {
		utils.Error(w, "Something went wrong.", http.StatusBadRequest, errors.Wrapf(err, "Failed to parse parameters for %s", r.URL.String()))
		return
	}
	rtc, err := runtime.GetConfig()
	if err != nil {
		utils.Error(w, "failed to get runtime config", http.StatusInternalServerError, errors.Wrap(err, "failed to get runtime config"))
		return
	}
	sc := image2.GetSystemContext(rtc.SignaturePolicyPath, "", false)
	tag := "latest"
	options := libpod.ContainerCommitOptions{
		Pause: true,
	}
	switch query.Format {
	case "oci":
		mimeType = buildah.OCIv1ImageManifest
		if len(query.Comment) > 0 {
			utils.InternalServerError(w, errors.New("messages are only compatible with the docker image format (-f docker)"))
			return
		}
	case "docker":
		mimeType = manifest.DockerV2Schema2MediaType
	default:
		utils.InternalServerError(w, errors.Errorf("unrecognized image format %q", query.Format))
		return
	}
	options.CommitOptions = buildah.CommitOptions{
		SignaturePolicyPath:   rtc.SignaturePolicyPath,
		ReportWriter:          os.Stderr,
		SystemContext:         sc,
		PreferredManifestType: mimeType,
	}

	if len(query.Tag) > 0 {
		tag = query.Tag
	}
	options.Message = query.Comment
	options.Author = query.Author
	options.Pause = query.Pause
	options.Changes = query.Changes
	ctr, err := runtime.LookupContainer(query.Container)
	if err != nil {
		utils.Error(w, "failed to lookup container", http.StatusNotFound, err)
		return
	}

	// I know mitr hates this ... but doing for now
	if len(query.Repo) > 1 {
		destImage = fmt.Sprintf("%s:%s", query.Repo, tag)
	}

	commitImage, err := ctr.Commit(r.Context(), destImage, options)
	if err != nil && !strings.Contains(err.Error(), "is not running") {
		utils.Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrapf(err, "CommitFailure"))
		return
	}
	utils.WriteResponse(w, http.StatusOK, handlers.IDResponse{ID: commitImage.ID()}) // nolint
}
