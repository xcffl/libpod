package server

import (
	"net/http"

	"github.com/containers/libpod/pkg/api/handlers/libpod"
	"github.com/gorilla/mux"
)

func (s *APIServer) registerManifestHandlers(r *mux.Router) error {
	// swagger:operation POST /libpod/manifests/create manifests Create
	// ---
	// summary: Create
	// description: Create a manifest list
	// produces:
	// - application/json
	// parameters:
	// - in: query
	//   name: name
	//   type: string
	//   description: manifest list name
	//   required: true
	// - in: query
	//   name: image
	//   type: string
	//   description: name of the image
	// - in: query
	//   name: all
	//   type: boolean
	//   description: add all contents if given list
	// responses:
	//   200:
	//     $ref: "#/definitions/IDResponse"
	//   400:
	//     $ref: "#/responses/BadParamError"
	//   404:
	//     $ref: "#/responses/NoSuchImage"
	//   500:
	//     $ref: "#/responses/InternalError"
	r.Handle(VersionedPath("/libpod/manifests/create"), s.APIHandler(libpod.ManifestCreate)).Methods(http.MethodPost)
	// swagger:operation GET /libpod/manifests/{name:.*}/json manifests Inspect
	// ---
	// summary: Inspect
	// description: Display a manifest list
	// produces:
	// - application/json
	// parameters:
	//  - in: path
	//    name: name:.*
	//    type: string
	//    required: true
	//    description: the name or ID of the manifest
	// responses:
	//   200:
	//     $ref: "#/responses/InspectManifest"
	//   404:
	//     $ref: "#/responses/NoSuchManifest"
	//   500:
	//     $ref: "#/responses/InternalError"
	r.Handle(VersionedPath("/libpod/manifests/{name:.*}/json"), s.APIHandler(libpod.ManifestInspect)).Methods(http.MethodGet)
	// swagger:operation POST /libpod/manifests/{name:.*}/add manifests AddManifest
	// ---
	// description: Add an image to a manifest list
	// produces:
	// - application/json
	// parameters:
	//  - in: path
	//    name: name:.*
	//    type: string
	//    required: true
	//    description: the name or ID of the manifest
	//  - in: body
	//    name: options
	//    description: options for creating a manifest
	//    schema:
	//      $ref: "#/definitions/ManifestAddOpts"
	// responses:
	//   200:
	//     $ref: "#/definitions/IDResponse"
	//   404:
	//     $ref: "#/responses/NoSuchManifest"
	//   409:
	//     $ref: "#/responses/BadParamError"
	//   500:
	//     $ref: "#/responses/InternalError"
	r.Handle(VersionedPath("/libpod/manifests/{name:.*}/add"), s.APIHandler(libpod.ManifestAdd)).Methods(http.MethodPost)
	// swagger:operation DELETE /libpod/manifests/{name:.*} manifests RemoveManifest
	// ---
	// summary: Remove
	// description: Remove an image from a manifest list
	// produces:
	// - application/json
	// parameters:
	//  - in: path
	//    name: name:.*
	//    type: string
	//    required: true
	//    description: the image associated with the manifest
	//  - in: query
	//    name: digest
	//    type: string
	//    description: image digest to be removed
	// responses:
	//   200:
	//     $ref: "#/definitions/IDResponse"
	//   400:
	//     $ref: "#/responses/BadParamError"
	//   404:
	//     $ref: "#/responses/NoSuchManifest"
	//   500:
	//     $ref: "#/responses/InternalError"
	r.Handle(VersionedPath("/libpod/manifests/{name:.*}"), s.APIHandler(libpod.ManifestRemove)).Methods(http.MethodDelete)
	// swagger:operation POST /libpod/manifests/{name}/push manifests PushManifest
	// ---
	// summary: Push
	// description: Push a manifest list or image index to a registry
	// produces:
	// - application/json
	// parameters:
	//  - in: path
	//    name: name
	//    type: string
	//    required: true
	//    description: the name or ID of the manifest
	//  - in: query
	//    name: destination
	//    type: string
	//    required: true
	//    description: the destination for the manifest
	//  - in: query
	//    name: all
	//    description: push all images
	//    type: boolean
	// responses:
	//   200:
	//     $ref: "#/definitions/IDResponse"
	//   400:
	//     $ref: "#/responses/BadParamError"
	//   404:
	//     $ref: "#/responses/NoSuchManifest"
	//   500:
	//     $ref: "#/responses/InternalError"
	r.Handle(VersionedPath("/libpod/manifests/{name}/push"), s.APIHandler(libpod.ManifestPush)).Methods(http.MethodPost)
	return nil
}
