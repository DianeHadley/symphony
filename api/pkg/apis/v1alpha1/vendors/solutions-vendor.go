/*
 * Copyright (c) Microsoft Corporation.
 * Licensed under the MIT license.
 * SPDX-License-Identifier: MIT
 */

package vendors

import (
	"encoding/json"

	"github.com/eclipse-symphony/symphony/api/pkg/apis/v1alpha1/managers/solutions"
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

var uLog = logger.NewLogger("coa.runtime")

type SolutionsVendor struct {
	vendors.Vendor
	SolutionsManager *solutions.SolutionsManager
}

func (o *SolutionsVendor) GetInfo() vendors.VendorInfo {
	return vendors.VendorInfo{
		Version:  o.Vendor.Version,
		Name:     "Solutions",
		Producer: "Microsoft",
	}
}

func (e *SolutionsVendor) Init(config vendors.VendorConfig, factories []managers.IManagerFactroy, providers map[string]map[string]providers.IProvider, pubsubProvider pubsub.IPubSubProvider) error {
	err := e.Vendor.Init(config, factories, providers, pubsubProvider)
	if err != nil {
		return err
	}
	for _, m := range e.Managers {
		if c, ok := m.(*solutions.SolutionsManager); ok {
			e.SolutionsManager = c
		}
	}
	if e.SolutionsManager == nil {
		return v1alpha2.NewCOAError(nil, "solutions manager is not supplied", v1alpha2.MissingConfig)
	}
	return nil
}

func (o *SolutionsVendor) GetEndpoints() []v1alpha2.Endpoint {
	route := "solutions"
	if o.Route != "" {
		route = o.Route
	}
	return []v1alpha2.Endpoint{
		{
			Methods:    []string{fasthttp.MethodGet, fasthttp.MethodPost, fasthttp.MethodDelete},
			Route:      route,
			Version:    o.Version,
			Handler:    o.onSolutions,
			Parameters: []string{"name?"},
		},
	}
}

func (c *SolutionsVendor) onSolutions(request v1alpha2.COARequest) v1alpha2.COAResponse {
	pCtx, span := observability.StartSpan("Solutions Vendor", request.Context, &map[string]string{
		"method": "onSolutions",
	})
	defer span.End()
	tLog.Info("V (Solutions): onSolutions")
	scope, exist := request.Parameters["scope"]
	if !exist {
		scope = "default"
	}
	switch request.Method {
	case fasthttp.MethodGet:
		ctx, span := observability.StartSpan("onSolutions-GET", pCtx, nil)
		id := request.Parameters["__name"]
		var err error
		var state interface{}
		isArray := false
		if id == "" {
			// Change scope back to empty to indicate ListSpec need to query all namespaces
			if !exist {
				scope = ""
			}
			state, err = c.SolutionsManager.ListSpec(ctx, scope)
			isArray = true
		} else {
			state, err = c.SolutionsManager.GetSpec(ctx, id, scope)
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
		ctx, span := observability.StartSpan("onSolutions-POST", pCtx, nil)
		id := request.Parameters["__name"]

		embed_type := request.Parameters["embed-type"]
		embed_component := request.Parameters["embed-component"]
		embed_property := request.Parameters["embed-property"]

		var solution model.SolutionSpec

		if embed_type != "" && embed_component != "" && embed_property != "" {
			solution = model.SolutionSpec{
				DisplayName: id,
				Components: []model.ComponentSpec{
					{
						Name: embed_component,
						Type: embed_type,
						Properties: map[string]interface{}{
							embed_property: string(request.Body),
						},
					},
				},
			}
		} else {
			err := json.Unmarshal(request.Body, &solution)
			if err != nil {
				return observ_utils.CloseSpanWithCOAResponse(span, v1alpha2.COAResponse{
					State: v1alpha2.InternalError,
					Body:  []byte(err.Error()),
				})
			}
		}
		err := c.SolutionsManager.UpsertSpec(ctx, id, solution, scope)
		if err != nil {
			return observ_utils.CloseSpanWithCOAResponse(span, v1alpha2.COAResponse{
				State: v1alpha2.InternalError,
				Body:  []byte(err.Error()),
			})
		}
		// TODO: this is a PoC of publishing trails when an object is updated
		c.Vendor.Context.Publish("trail", v1alpha2.Event{
			Body: []v1alpha2.Trail{
				{
					Origin:  c.Vendor.Context.SiteInfo.SiteId,
					Catalog: solution.Metadata["catalog"],
					Type:    "solutions.solution.symphony/v1",
					Properties: map[string]interface{}{
						"spec": solution,
					},
				},
			},
			Metadata: map[string]string{
				"scope": scope,
			},
		})
		return observ_utils.CloseSpanWithCOAResponse(span, v1alpha2.COAResponse{
			State: v1alpha2.OK,
		})
	case fasthttp.MethodDelete:
		ctx, span := observability.StartSpan("onSolutions-DELETE", pCtx, nil)
		id := request.Parameters["__name"]
		err := c.SolutionsManager.DeleteSpec(ctx, id, scope)
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
