/*
 * Copyright (c) Microsoft Corporation.
 * Licensed under the MIT license.
 * SPDX-License-Identifier: MIT
 */

package vendors

import (
	"encoding/json"

	"github.com/eclipse-symphony/symphony/api/pkg/apis/v1alpha1/managers/activations"
	"github.com/eclipse-symphony/symphony/api/pkg/apis/v1alpha1/model"
	"github.com/eclipse-symphony/symphony/api/pkg/apis/v1alpha1/utils"
	"github.com/eclipse-symphony/symphony/coa/pkg/apis/v1alpha2"
	"github.com/eclipse-symphony/symphony/coa/pkg/apis/v1alpha2/managers"
	"github.com/eclipse-symphony/symphony/coa/pkg/apis/v1alpha2/observability"
	observ_utils "github.com/eclipse-symphony/symphony/coa/pkg/apis/v1alpha2/observability/utils"
	"github.com/eclipse-symphony/symphony/coa/pkg/apis/v1alpha2/providers"
	"github.com/eclipse-symphony/symphony/coa/pkg/apis/v1alpha2/providers/pubsub"
	"github.com/eclipse-symphony/symphony/coa/pkg/apis/v1alpha2/vendors"
	"github.com/eclipse-symphony/symphony/coa/pkg/logger"
	"github.com/valyala/fasthttp"
)

var vLog = logger.NewLogger("coa.runtime")

type ActivationsVendor struct {
	vendors.Vendor
	ActivationsManager *activations.ActivationsManager
}

func (o *ActivationsVendor) GetInfo() vendors.VendorInfo {
	return vendors.VendorInfo{
		Version:  o.Vendor.Version,
		Name:     "Activations",
		Producer: "Microsoft",
	}
}

func (e *ActivationsVendor) Init(config vendors.VendorConfig, factories []managers.IManagerFactroy, providers map[string]map[string]providers.IProvider, pubsubProvider pubsub.IPubSubProvider) error {
	err := e.Vendor.Init(config, factories, providers, pubsubProvider)
	if err != nil {
		return err
	}
	for _, m := range e.Managers {
		if c, ok := m.(*activations.ActivationsManager); ok {
			e.ActivationsManager = c
		}
	}
	if e.ActivationsManager == nil {
		return v1alpha2.NewCOAError(nil, "activations manager is not supplied", v1alpha2.MissingConfig)
	}
	return nil
}

func (o *ActivationsVendor) GetEndpoints() []v1alpha2.Endpoint {
	route := "activations"
	if o.Route != "" {
		route = o.Route
	}
	return []v1alpha2.Endpoint{
		{
			Methods:    []string{fasthttp.MethodGet, fasthttp.MethodPost, fasthttp.MethodDelete},
			Route:      route + "/registry",
			Version:    o.Version,
			Handler:    o.onActivations,
			Parameters: []string{"name?"},
		},
		{
			Methods:    []string{fasthttp.MethodPost},
			Route:      route + "/status",
			Version:    o.Version,
			Handler:    o.onStatus,
			Parameters: []string{"name?"},
		},
	}
}

func (c *ActivationsVendor) onStatus(request v1alpha2.COARequest) v1alpha2.COAResponse {
	pCtx, span := observability.StartSpan("Activations Vendor", request.Context, &map[string]string{
		"method": "onStatus",
	})
	defer span.End()

	cLog.Info("V (Activations Vendor): onStatus")
	switch request.Method {
	case fasthttp.MethodPost:
		ctx, span := observability.StartSpan("onStatus-POST", pCtx, nil)
		id := request.Parameters["__name"]
		var status model.ActivationStatus
		err := json.Unmarshal(request.Body, &status)
		if err != nil {
			return observ_utils.CloseSpanWithCOAResponse(span, v1alpha2.COAResponse{
				State: v1alpha2.InternalError,
				Body:  []byte(err.Error()),
			})
		}
		err = c.ActivationsManager.ReportStatus(ctx, id, status)
		if err != nil {
			return observ_utils.CloseSpanWithCOAResponse(span, v1alpha2.COAResponse{
				State: v1alpha2.InternalError,
				Body:  []byte(err.Error()),
			})
		}
		return observ_utils.CloseSpanWithCOAResponse(span, v1alpha2.COAResponse{
			State: v1alpha2.OK,
		})
	}
	resp := v1alpha2.COAResponse{
		State:       v1alpha2.MethodNotAllowed,
		Body:        []byte("{\"result\":\"405 - method not allowed\"}"),
		ContentType: "application/json",
	}
	observ_utils.UpdateSpanStatusFromCOAResponse(span, resp)
	return resp
}
func (c *ActivationsVendor) onActivations(request v1alpha2.COARequest) v1alpha2.COAResponse {
	pCtx, span := observability.StartSpan("Activations Vendor", request.Context, &map[string]string{
		"method": "onActivations",
	})
	defer span.End()

	cLog.Info("V (Activations Vendor): onActivations")

	switch request.Method {
	case fasthttp.MethodGet:
		ctx, span := observability.StartSpan("onActivations-GET", pCtx, nil)
		id := request.Parameters["__name"]
		var err error
		var state interface{}
		isArray := false
		if id == "" {
			state, err = c.ActivationsManager.ListSpec(ctx)
			isArray = true
		} else {
			state, err = c.ActivationsManager.GetSpec(ctx, id)
		}
		if err != nil {
			return observ_utils.CloseSpanWithCOAResponse(span, v1alpha2.COAResponse{
				State: v1alpha2.InternalError,
				Body:  []byte(err.Error()),
			})
		}
		jData, _ := utils.FormatObject(state, isArray, request.Parameters["path"], request.Parameters["doc-type"])
		resp := observ_utils.CloseSpanWithCOAResponse(span, v1alpha2.COAResponse{
			State:       v1alpha2.OK,
			Body:        jData,
			ContentType: "application/json",
		})
		if request.Parameters["doc-type"] == "yaml" {
			resp.ContentType = "application/text"
		}
		return resp
	case fasthttp.MethodPost:
		ctx, span := observability.StartSpan("onActivations-POST", pCtx, nil)
		id := request.Parameters["__name"]

		var activation model.ActivationSpec

		err := json.Unmarshal(request.Body, &activation)
		if err != nil {
			return observ_utils.CloseSpanWithCOAResponse(span, v1alpha2.COAResponse{
				State: v1alpha2.InternalError,
				Body:  []byte(err.Error()),
			})
		}

		err = c.ActivationsManager.UpsertSpec(ctx, id, activation)
		if err != nil {
			return observ_utils.CloseSpanWithCOAResponse(span, v1alpha2.COAResponse{
				State: v1alpha2.InternalError,
				Body:  []byte(err.Error()),
			})
		}
		entry, err := c.ActivationsManager.GetSpec(ctx, id)
		if err != nil {
			return observ_utils.CloseSpanWithCOAResponse(span, v1alpha2.COAResponse{
				State: v1alpha2.InternalError,
				Body:  []byte(err.Error()),
			})
		}
		c.Context.Publish("activation", v1alpha2.Event{
			Body: v1alpha2.ActivationData{
				Campaign:             activation.Campaign,
				ActivationGeneration: entry.Spec.Generation,
				Activation:           id,
				Stage:                "",
				Inputs:               activation.Inputs,
			},
		})
		return observ_utils.CloseSpanWithCOAResponse(span, v1alpha2.COAResponse{
			State: v1alpha2.OK,
		})
	case fasthttp.MethodDelete:
		ctx, span := observability.StartSpan("onActivations-DELETE", pCtx, nil)
		id := request.Parameters["__name"]
		err := c.ActivationsManager.DeleteSpec(ctx, id)
		if err != nil {
			return observ_utils.CloseSpanWithCOAResponse(span, v1alpha2.COAResponse{
				State: v1alpha2.InternalError,
				Body:  []byte(err.Error()),
			})
		}
		return observ_utils.CloseSpanWithCOAResponse(span, v1alpha2.COAResponse{
			State: v1alpha2.OK,
		})
	}
	resp := v1alpha2.COAResponse{
		State:       v1alpha2.MethodNotAllowed,
		Body:        []byte("{\"result\":\"405 - method not allowed\"}"),
		ContentType: "application/json",
	}
	observ_utils.UpdateSpanStatusFromCOAResponse(span, resp)
	return resp
}
