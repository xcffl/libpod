package filters

import (
	"strings"

	"github.com/containers/libpod/libpod"
	"github.com/pkg/errors"
)

func GenerateVolumeFilters(filters map[string][]string) ([]libpod.VolumeFilter, error) {
	var vf []libpod.VolumeFilter
	for filter, v := range filters {
		for _, val := range v {
			switch filter {
			case "name":
				nameVal := val
				vf = append(vf, func(v *libpod.Volume) bool {
					return nameVal == v.Name()
				})
			case "driver":
				driverVal := val
				vf = append(vf, func(v *libpod.Volume) bool {
					return v.Driver() == driverVal
				})
			case "scope":
				scopeVal := val
				vf = append(vf, func(v *libpod.Volume) bool {
					return v.Scope() == scopeVal
				})
			case "label":
				filterArray := strings.SplitN(val, "=", 2)
				filterKey := filterArray[0]
				var filterVal string
				if len(filterArray) > 1 {
					filterVal = filterArray[1]
				} else {
					filterVal = ""
				}
				vf = append(vf, func(v *libpod.Volume) bool {
					for labelKey, labelValue := range v.Labels() {
						if labelKey == filterKey && ("" == filterVal || labelValue == filterVal) {
							return true
						}
					}
					return false
				})
			case "opt":
				filterArray := strings.SplitN(val, "=", 2)
				filterKey := filterArray[0]
				var filterVal string
				if len(filterArray) > 1 {
					filterVal = filterArray[1]
				} else {
					filterVal = ""
				}
				vf = append(vf, func(v *libpod.Volume) bool {
					for labelKey, labelValue := range v.Options() {
						if labelKey == filterKey && ("" == filterVal || labelValue == filterVal) {
							return true
						}
					}
					return false
				})
			default:
				return nil, errors.Errorf("%q is in an invalid volume filter", filter)
			}
		}
	}
	return vf, nil
}
