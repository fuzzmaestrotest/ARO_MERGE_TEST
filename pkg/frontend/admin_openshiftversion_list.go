package frontend

// Copyright (c) Microsoft Corporation.
// Licensed under the Apache License 2.0.

import (
	"encoding/json"
	"net/http"
	"path/filepath"

	"github.com/sirupsen/logrus"

	"github.com/Azure/ARO-RP/pkg/api"
	"github.com/Azure/ARO-RP/pkg/api/admin"
	"github.com/Azure/ARO-RP/pkg/frontend/middleware"
)

func (f *frontend) getAdminOpenShiftVersions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := ctx.Value(middleware.ContextKeyLog).(*logrus.Entry)
	r.URL.Path = filepath.Dir(r.URL.Path)

	converter := f.apis[admin.APIVersion].OpenShiftVersionConverter()

	docs, err := f.dbOpenShiftVersions.ListAll(ctx)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CloudErrorCodeInternalServerError, "", "Internal server error.")
		return
	}

	var vers []*api.OpenShiftVersion
	if docs != nil {
		for _, doc := range docs.OpenShiftVersionDocuments {
			vers = append(vers, doc.OpenShiftVersion)
		}
	}

	b, err := json.MarshalIndent(converter.ToExternalList(vers), "", "    ")
	adminReply(log, w, nil, b, err)
}
