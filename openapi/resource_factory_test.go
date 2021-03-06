package openapi

import (
	"errors"
	"fmt"
	"github.com/dikhan/terraform-provider-openapi/openapi/openapierr"
	"github.com/go-openapi/spec"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"testing"
	"time"

	"encoding/json"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	. "github.com/smartystreets/goconvey/convey"
)

func TestCreateSchemaResourceTimeout(t *testing.T) {
	Convey("Given a resource factory initialised with a spec resource that has some timeouts", t, func() {
		duration, _ := time.ParseDuration("30m")
		expectedTimeouts := &specTimeouts{
			Get:    &duration,
			Post:   &duration,
			Put:    &duration,
			Delete: &duration,
		}
		r := newResourceFactory(&specStubResource{
			timeouts: expectedTimeouts,
		})
		Convey("When createSchemaResourceTimeout is called", func() {
			timeouts, err := r.createSchemaResourceTimeout()
			Convey("Then the error returned should be nil", func() {
				So(err, ShouldBeNil)
			})
			Convey("And the timeouts should match the expected ones", func() {
				So(timeouts.Read, ShouldEqual, expectedTimeouts.Get)
				So(timeouts.Create, ShouldEqual, expectedTimeouts.Post)
				So(timeouts.Delete, ShouldEqual, expectedTimeouts.Delete)
				So(timeouts.Update, ShouldEqual, expectedTimeouts.Put)
			})
		})
	})
}

func TestCreateTerraformResource(t *testing.T) {
	Convey("Given a resource factory initialised with a spec resource that has an id and string property and supports all CRUD operations", t, func() {
		r, resourceData := testCreateResourceFactory(t, idProperty, stringProperty)
		Convey("When createTerraformResource is called", func() {
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					idProperty.Name:     idProperty.Default,
					stringProperty.Name: stringProperty.Default,
				},
			}
			schemaResource, err := r.createTerraformResource()
			Convey("Then the error returned should be nil", func() {
				So(err, ShouldBeNil)
			})
			Convey("And the schema resource should not be empty", func() {
				So(schemaResource.Schema, ShouldNotBeEmpty)
			})
			Convey("And the create function is invokable and returns nil error", func() {
				err := schemaResource.Create(resourceData, client)
				So(err, ShouldBeNil)
			})
			Convey("And the read function is invokable and returns nil error", func() {
				err := schemaResource.Read(resourceData, client)
				So(err, ShouldBeNil)
			})
			Convey("And the update function is invokable and returns nil error", func() {
				err := schemaResource.Update(resourceData, client)
				So(err, ShouldBeNil)
			})
			Convey("And the delete function is invokable and returns nil error", func() {
				err := schemaResource.Delete(resourceData, client)
				So(err, ShouldBeNil)
			})
		})
	})
}

func TestCreateTerraformResourceSchema(t *testing.T) {
	Convey("Given a resource factory", t, func() {
		r, _ := testCreateResourceFactory(t, idProperty, stringProperty)
		Convey("When createResourceSchema is called", func() {
			schema, err := r.createTerraformResourceSchema()
			Convey("Then the error returned should be nil", func() {
				So(err, ShouldBeNil)
			})
			Convey("And the schema returned should not contain the ID property as schema already has a reserved ID field to store the unique identifier", func() {
				So(schema, ShouldNotContainKey, idProperty.Name)
			})
			Convey("And the schema returned should contain the resource properties", func() {
				So(schema, ShouldContainKey, stringProperty.Name)
			})
		})
	})
}

func TestCreate(t *testing.T) {
	Convey("Given a resource factory", t, func() {
		var telemetryHandlerResourceNameReceived string
		var telemetryHandlerTFOperationReceived TelemetryResourceOperation
		r, resourceData := testCreateResourceFactory(t, idProperty, stringProperty)
		Convey("When create is called with resource data and a client", func() {
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					idProperty.Name:     "someID",
					stringProperty.Name: "someExtraValueThatProvesResponseDataIsPersisted",
				},
				telemetryHandler: &telemetryHandlerStub{
					submitResourceExecutionMetricsFunc: func(resourceName string, tfOperation TelemetryResourceOperation) {
						telemetryHandlerResourceNameReceived = resourceName
						telemetryHandlerTFOperationReceived = tfOperation
					},
				},
			}
			err := r.create(resourceData, client)
			Convey("Then the error returned should be nil", func() {
				So(err, ShouldBeNil)
			})
			Convey("And resourceData should be populated with the values returned by the API including the ID", func() {
				So(resourceData.Id(), ShouldEqual, client.responsePayload[idProperty.Name])
				So(resourceData.Get(stringProperty.Name), ShouldEqual, client.responsePayload[stringProperty.Name])
			})
			Convey("And the expected telemetry provider should have been called", func() {
				So(telemetryHandlerResourceNameReceived, ShouldEqual, "resourceName")
				So(telemetryHandlerTFOperationReceived, ShouldEqual, TelemetryResourceOperationCreate)
			})
		})
		Convey("When create is called with resource data and a client configured to return an error when POST is called", func() {
			createError := fmt.Errorf("some error when deleting")
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					idProperty.Name: idProperty.Default,
				},
				error: createError,
			}
			err := r.create(resourceData, client)
			Convey("Then the error returned should not be nil", func() {
				So(err, ShouldNotBeNil)
			})
			Convey("Then the error returned should be the error returned by the client delete operation", func() {
				So(err, ShouldEqual, createError)
			})
		})

		Convey("When update is called with resource data and a client returns a non expected http code", func() {
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{},
				returnHTTPCode:  http.StatusInternalServerError,
			}
			err := r.create(resourceData, client)
			Convey("Then the error returned should NOT be nil", func() {
				So(err, ShouldNotBeNil)
			})
			Convey("And the error returned should be", func() {
				So(err.Error(), ShouldEqual, "[resource='resourceName'] POST /v1/resource failed: [resource='resourceName'] HTTP Response Status Code 500 not matching expected one [200 201 202] ()")
			})
		})

		Convey("When update is called with resource data and a client returns a response that does not have an id property", func() {
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{},
			}
			err := r.create(resourceData, client)
			Convey("Then the error returned should NOT be nil", func() {
				So(err, ShouldNotBeNil)
			})
			Convey("And the error returned should be", func() {
				So(err.Error(), ShouldEqual, "response object returned from the API is missing mandatory identifier property 'id'")
			})
		})
	})

	Convey("Given an empty resource factory with an empty OpenAPI resource", t, func() {
		r := resourceFactory{}
		Convey("When create is called with empty data and a empty client", func() {
			client := &clientOpenAPIStub{}
			err := r.create(nil, client)
			Convey("Then the error returned should be the expected one", func() {
				So(err.Error(), ShouldEqual, "missing openAPI resource configuration")
			})
		})
	})

	Convey("Given a resource factory that has an asynchronous create operation (post) but the polling operation fails for some reason", t, func() {
		expectedReturnCode := 202
		testSchema := newTestSchema(idProperty, stringProperty)
		resourceData := testSchema.getResourceData(t)
		specResource := newSpecStubResourceWithOperations("resourceName", "/v1/resource", false, testSchema.getSchemaDefinition(), &specResourceOperation{responses: specResponses{expectedReturnCode: &specResponse{isPollingEnabled: true}}}, &specResourceOperation{}, &specResourceOperation{}, &specResourceOperation{})
		r := resourceFactory{
			openAPIResource: specResource,
			defaultTimeout:  time.Duration(0 * time.Second),
		}
		Convey("When create is called with resource data and a client", func() {
			client := &clientOpenAPIStub{
				returnHTTPCode: expectedReturnCode,
				responsePayload: map[string]interface{}{
					idProperty.Name:     "someID",
					stringProperty.Name: "someExtraValueThatProvesResponseDataIsPersisted",
				},
			}
			err := r.create(resourceData, client)
			Convey("Then the error returned should be the expected one", func() {
				So(err.Error(), ShouldEqual, "polling mechanism failed after POST /v1/resource call with response status code (202): error waiting for resource to reach a completion status ([]) [valid pending statuses ([])]: error on retrieving resource 'resourceName' (someID) when waiting: [resource='resourceName'] HTTP Response Status Code 202 not matching expected one [200] ()")
			})
		})
	})

	Convey("Given a resource factory where getResourcePath returns an error", t, func() {
		r := resourceFactory{
			openAPIResource: &specStubResource{
				funcGetResourcePath: func(parentIDs []string) (s string, e error) {
					return "", errors.New("getResourcePath() failed")
				}},
		}
		Convey("When create is called with resource data and an empty clientOpenAPI", func() {
			err := r.create(&schema.ResourceData{}, &clientOpenAPIStub{})
			Convey("Then the error returned should be the expected one", func() {
				assert.EqualError(t, err, "getResourcePath() failed")
			})
		})
	})
}

func TestReadWithOptions(t *testing.T) {
	Convey("Given a resource factory and an OpenAPI client that returns a responsePayload", t, func() {
		var telemetryHandlerResourceNameReceived string
		var telemetryHandlerTFOperationReceived TelemetryResourceOperation
		r, resourceData := testCreateResourceFactory(t, idProperty, stringProperty)
		client := &clientOpenAPIStub{
			responsePayload: map[string]interface{}{
				stringProperty.Name: "someOtherStringValue",
			},
			telemetryHandler: &telemetryHandlerStub{
				submitResourceExecutionMetricsFunc: func(resourceName string, tfOperation TelemetryResourceOperation) {
					telemetryHandlerResourceNameReceived = resourceName
					telemetryHandlerTFOperationReceived = tfOperation
				},
			},
		}
		Convey("When readWithOptions is called with handleNotFound set to false", func() {
			err := r.readWithOptions(resourceData, client, false)
			Convey("Then resourceData should equal the responsePayload and the expected telemetry provider should be called ", func() {
				So(err, ShouldBeNil)
				So(resourceData.Get(stringProperty.Name), ShouldEqual, client.responsePayload[stringProperty.Name])
				So(telemetryHandlerResourceNameReceived, ShouldEqual, "resourceName")
				So(telemetryHandlerTFOperationReceived, ShouldEqual, TelemetryResourceOperationRead)
			})
		})
	})

	Convey("Given a resource factory configured with a subresource and an OpenAPI client that returns a responsePayload", t, func() {
		someOtherProperty := newStringSchemaDefinitionPropertyWithDefaults("some_string_prop", "", true, false, "some value")
		parentProperty := newStringSchemaDefinitionPropertyWithDefaults("cdns_v1_id", "", true, false, "parentPropertyID")
		// Pretending the data has already been populated with the parent property
		testSchema := newTestSchema(idProperty, someOtherProperty, parentProperty)
		resourceData := testSchema.getResourceData(t)
		r := newResourceFactory(&SpecV2Resource{
			Path: "/v1/cdns/{id}/firewall",
			SchemaDefinition: spec.Schema{
				SchemaProps: spec.SchemaProps{
					Properties: map[string]spec.Schema{
						"some_string_prop": {
							SchemaProps: spec.SchemaProps{
								Type: []string{"string"},
							},
						},
					},
				},
			},
		})
		client := &clientOpenAPIStub{
			responsePayload: map[string]interface{}{
				someOtherProperty.Name: "someOtherStringValue",
			},
		}
		Convey("When readWithOptions is called with handleNotFound set to false", func() {
			err := r.readWithOptions(resourceData, client, false)
			Convey("Then resourceData should equal the responsePayload", func() {
				So(err, ShouldBeNil)
				So(resourceData.Get(stringProperty.Name), ShouldEqual, client.responsePayload[stringProperty.Name])
			})
		})
	})

	Convey("Given a resource factory with an empty OpenAPI resource and an empty OpenAPI client", t, func() {
		r := resourceFactory{}
		client := &clientOpenAPIStub{}
		Convey("When readWithOptions is called with nil data, an empty clientOpenAPI, and handleNotFound set to false", func() {
			err := r.readWithOptions(nil, client, false)
			Convey("Then the error returned should be the expected one", func() {
				So(err.Error(), ShouldEqual, "missing openAPI resource configuration")
			})
		})
	})

	Convey("Given a resource factory where getResourcePath returns an error", t, func() {
		r := resourceFactory{
			openAPIResource: &specStubResource{
				funcGetResourcePath: func(parentIDs []string) (s string, e error) {
					return "", errors.New("getResourcePath() failed")
				}},
		}
		Convey("When readWithOptions is called with nil data, an empty clientOpenAPI, and handleNotFound set to false", func() {
			err := r.readWithOptions(&schema.ResourceData{}, &clientOpenAPIStub{}, false)
			Convey("Then the error returned should be the expected one", func() {
				assert.EqualError(t, err, "getResourcePath() failed")
			})
		})
	})

	Convey("Given a resource factory with a clientOpenAPI where GET returns an NotFound error", t, func() {
		r, resourceData := testCreateResourceFactory(t, idProperty, stringProperty)
		c := &clientOpenAPIStub{
			error: &openapierr.NotFoundError{
				OriginalError: errors.New(openapierr.NotFound),
			},
		}
		Convey("When readWithOptions is called with handleNotFound set to true", func() {
			err := r.readWithOptions(resourceData, c, true)
			Convey("Then the error returned should be the expected one", func() {
				assert.EqualError(t, err, "[resource='resourceName'] GET /v1/resource/ failed: NotFound")
			})
		})
		Convey("When readWithOptions is called with handleNotFound set to false", func() {
			err := r.readWithOptions(resourceData, c, false)
			Convey("Then the error returned should be nil", func() {
				assert.Nil(t, err)
			})
		})
	})

	Convey("Given a resource factory with a clientOpenAPI where GET returns an error (that isn't NotFound)", t, func() {
		r, resourceData := testCreateResourceFactory(t, idProperty, stringProperty)
		c := &clientOpenAPIStub{
			error: errors.New("some generic error"),
		}
		Convey("When readWithOptions is called with handleNotFound set to true", func() {
			err := r.readWithOptions(resourceData, c, true)
			Convey("Then the error returned should be the expected one", func() {
				assert.EqualError(t, err, "[resource='resourceName'] GET /v1/resource/ failed: some generic error")
			})
		})
		Convey("When readWithOptions is called with handleNotFound set to false", func() {
			err := r.readWithOptions(resourceData, c, false)
			Convey("Then the error returned should be nil", func() {
				assert.EqualError(t, err, "[resource='resourceName'] GET /v1/resource/ failed: some generic error")
			})
		})
	})
}

func TestGetParentID(t *testing.T) {
	expectedParentID := "32"
	resourceParentName := "cdns_v1"
	expectedParentPropertyName := fmt.Sprintf("%s_id", resourceParentName)
	expectedResourceInstanceID := "159"
	IDValue := fmt.Sprintf("%s/%s", expectedParentID, expectedResourceInstanceID)
	IDProperty := newStringSchemaDefinitionProperty("id", "", true, true, false, false, false, true, false, false, IDValue)
	expectedParentProperty := newStringSchemaDefinitionProperty(expectedParentPropertyName, "", true, true, false, false, false, true, false, false, expectedParentID)
	r, resourceData := testCreateSubResourceFactory(t, "/v1/cdns/{id}/firewall", []string{resourceParentName}, "cdns_v1", IDProperty, expectedParentProperty)
	parentIDs, err := r.getParentIDs(resourceData)
	assert.NoError(t, err)
	assert.Equal(t, expectedParentID, parentIDs[0])
}

func TestGetParentID_MissingOpenAPIResource(t *testing.T) {
	d := &schema.ResourceData{}
	r := resourceFactory{openAPIResource: nil}
	parentIDs, err := r.getParentIDs(d)
	assert.Empty(t, parentIDs)
	assert.EqualError(t, err, "can't get parent ids from a resourceFactory with no openAPIResource")
}

func TestGetParentID_MissingParentPropID(t *testing.T) {
	d := &schema.ResourceData{}
	expectedParentProperty := &SpecSchemaDefinitionProperty{Name: "cdns_v1_id", Required: true, Type: TypeString, Default: "32"}
	testSchema := &testSchemaDefinition{expectedParentProperty}
	specResource := &specStubResource{
		name:                   "firewall",
		path:                   "/v1/cdns/{id}/firewall",
		schemaDefinition:       testSchema.getSchemaDefinition(),
		parentResourceNames:    []string{"cdns_v1"},
		fullParentResourceName: "cdns_v1",
	}
	r := newResourceFactory(specResource)
	parentIDs, err := r.getParentIDs(d)
	assert.Empty(t, parentIDs)
	assert.EqualError(t, err, "could not find ID value in the state file for subresource parent property 'cdns_v1_id'")
}

func TestReadRemote(t *testing.T) {

	Convey("Given a resource factory", t, func() {
		r := newResourceFactory(&specStubResource{name: "resourceName"})
		Convey("When readRemote is called with resource data and a client that returns ", func() {
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					idProperty.Name:     "someID",
					stringProperty.Name: "someOtherStringValue",
				},
			}
			response, err := r.readRemote("", client)
			Convey("Then the error returned should be nil", func() {
				So(err, ShouldBeNil)
			})
			Convey("And the map returned should be contain the properties in the response payload", func() {
				So(response, ShouldContainKey, idProperty.Name)
				So(response, ShouldContainKey, stringProperty.Name)
			})
			Convey("And the values of the keys should match the values that came in the response", func() {
				So(response[idProperty.Name], ShouldEqual, client.responsePayload[idProperty.Name])
				So(response[stringProperty.Name], ShouldEqual, client.responsePayload[stringProperty.Name])
			})
		})

		Convey("When readRemote is called with resource data and a client returns a non expected http code", func() {
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{},
				returnHTTPCode:  http.StatusInternalServerError,
			}
			_, err := r.readRemote("", client)
			Convey("Then the error returned should NOT be nil", func() {
				So(err, ShouldNotBeNil)
			})
			Convey("And the error returned should be", func() {
				So(err.Error(), ShouldEqual, "[resource='resourceName'] HTTP Response Status Code 500 not matching expected one [200] ()")
			})
		})

		Convey("When readRemote is called with an instance id, a providerClient and a parent ID", func() {
			expectedID := "someID"
			expectedParentID := "someParentID"
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					idProperty.Name:     "someID",
					stringProperty.Name: "someOtherStringValue",
				},
			}
			response, err := r.readRemote(expectedID, client, expectedParentID)
			Convey("Then the error returned should be nil", func() {
				So(err, ShouldBeNil)
			})
			Convey("And the provider client should have been called with the right argument values", func() {
				So(client.idReceived, ShouldEqual, expectedID)
				So(len(client.parentIDsReceived), ShouldEqual, 1)
				So(client.parentIDsReceived[0], ShouldEqual, expectedParentID)
			})
			Convey("And the values of the keys should match the values that came in the response", func() {
				So(response[idProperty.Name], ShouldEqual, client.responsePayload[idProperty.Name])
				So(response[stringProperty.Name], ShouldEqual, client.responsePayload[stringProperty.Name])
			})
		})
	})
}

func TestUpdate(t *testing.T) {
	Convey("Given a resource factory containing some properties including an immutable property", t, func() {
		var telemetryHandlerResourceNameReceived string
		var telemetryHandlerTFOperationReceived TelemetryResourceOperation
		r, resourceData := testCreateResourceFactoryWithID(t, idProperty, stringProperty, immutableProperty)
		Convey("When update is called with resource data and a client", func() {
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					idProperty.Name:        "id",
					stringProperty.Name:    "someExtraValueThatProvesResponseDataIsPersisted",
					immutableProperty.Name: immutableProperty.Default,
				},
				telemetryHandler: &telemetryHandlerStub{
					submitResourceExecutionMetricsFunc: func(resourceName string, tfOperation TelemetryResourceOperation) {
						telemetryHandlerResourceNameReceived = resourceName
						telemetryHandlerTFOperationReceived = tfOperation
					},
				},
			}
			err := r.update(resourceData, client)
			Convey("Then the error returned should be nil", func() {
				So(err, ShouldBeNil)
			})
			Convey("And resourceData should contain the ID property", func() {
				So(resourceData.Id(), ShouldEqual, idProperty.Default)
			})
			Convey("And resourceData should be populated with the values returned by the API", func() {
				So(resourceData.Get(stringProperty.Name), ShouldEqual, client.responsePayload[stringProperty.Name])
				So(resourceData.Get(immutableProperty.Name), ShouldEqual, client.responsePayload[immutableProperty.Name])
			})
			Convey("And the expected telemetry provider should have been called", func() {
				So(telemetryHandlerResourceNameReceived, ShouldEqual, "resourceName")
				So(telemetryHandlerTFOperationReceived, ShouldEqual, TelemetryResourceOperationUpdate)
			})
		})
		Convey("When update is called with a resource data containing updated values and the immutable check fails due to an immutable property being updated", func() {
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					idProperty.Name:        "id",
					stringProperty.Name:    "stringOriginalValue",
					immutableProperty.Name: "immutableOriginalValue",
				},
			}
			err := r.update(resourceData, client)
			Convey("Then the error returned should be nil", func() {
				So(err, ShouldNotBeNil)
			})
			Convey("And the error returned should equal ", func() {
				So(err.Error(), ShouldEqual, "validation for immutable properties failed: user attempted to update an immutable property ('string_immutable_property'): [user input: updatedImmutableValue; actual: immutableOriginalValue]. Update operation was aborted; no updates were performed")
			})
			Convey("And resourceData values should be the values got from the response payload (original values)", func() {
				So(resourceData.Id(), ShouldEqual, idProperty.Default)
				So(resourceData.Get(stringProperty.Name), ShouldEqual, client.responsePayload[stringProperty.Name])
				So(resourceData.Get(immutableProperty.Name), ShouldEqual, client.responsePayload[immutableProperty.Name])
			})
		})
		Convey("When update is called with resource data and a client configured to return an error when update is called", func() {
			updateError := fmt.Errorf("some error when deleting")
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					idProperty.Name: idProperty.Default,
				},
				error: updateError,
			}
			err := r.update(resourceData, client)
			Convey("Then the error returned should not be nil", func() {
				So(err, ShouldNotBeNil)
			})
			Convey("Then the error returned should be the error returned by the client update operation", func() {
				So(err, ShouldEqual, updateError)
			})
		})
	})

	Convey("Given a resource factory containing some properties", t, func() {
		r, resourceData := testCreateResourceFactoryWithID(t, idProperty, stringProperty)
		Convey("When update is called with resource data and a client returns a non expected http code when reading remote", func() {
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					idProperty.Name:     "id",
					stringProperty.Name: "someExtraValueThatProvesResponseDataIsPersisted",
				},
				funcPut: func() (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusInternalServerError,
						Body:       ioutil.NopCloser(strings.NewReader("")),
					}, nil
				},
			}
			err := r.update(resourceData, client)
			Convey("And the error returned should be the expected one", func() {
				So(err.Error(), ShouldEqual, "[resource='resourceName'] UPDATE /v1/resource/id failed: [resource='resourceName'] HTTP Response Status Code 500 not matching expected one [200 202] ()")
			})
		})
		Convey("When update is called with resource data and a client returns a non expected error", func() {
			expectedError := "some error returned by the PUT operation"
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					idProperty.Name:     "id",
					stringProperty.Name: "someValue",
				},
				funcPut: func() (*http.Response, error) {
					return nil, fmt.Errorf(expectedError)
				},
			}
			err := r.update(resourceData, client)
			Convey("And the error returned should be the expected one", func() {
				So(err.Error(), ShouldEqual, expectedError)
			})
		})
	})

	Convey("Given a resource factory with no update operation configured", t, func() {
		specResource := newSpecStubResource("resourceName", "/v1/resource", false, nil)
		r := newResourceFactory(specResource)
		Convey("When update is called with resource data and a client", func() {
			client := &clientOpenAPIStub{}
			err := r.update(&schema.ResourceData{}, client)
			Convey("Then the expectedValue returned should be true", func() {
				So(err, ShouldNotBeNil)
			})
			Convey("And resourceData should be populated with the values returned by the API including the ID", func() {
				So(err.Error(), ShouldEqual, "[resource='resourceName'] resource does not support PUT operation, check the swagger file exposed on '/v1/resource'")
			})
		})
	})

	Convey("Given a resource factory with an empty OpenAPI resource", t, func() {
		r := resourceFactory{}
		Convey("When create is called with empty data and a empty client", func() {
			client := &clientOpenAPIStub{}
			err := r.update(nil, client)
			Convey("Then the error returned should be the expected one", func() {
				So(err.Error(), ShouldEqual, "missing openAPI resource configuration")
			})
		})
	})

	Convey("Given a resource factory that has an asynchronous create operation (put) but the polling operation fails due to the status field missing", t, func() {
		expectedReturnCode := 202
		testSchema := newTestSchema(idProperty, stringProperty)
		resourceData := testSchema.getResourceData(t)
		specResource := newSpecStubResourceWithOperations("resourceName", "/v1/resource", false, testSchema.getSchemaDefinition(), &specResourceOperation{}, &specResourceOperation{responses: specResponses{expectedReturnCode: &specResponse{isPollingEnabled: true}}}, &specResourceOperation{}, &specResourceOperation{})
		r := resourceFactory{
			openAPIResource: specResource,
			defaultTimeout:  time.Duration(0 * time.Second),
		}
		Convey("When create is called with resource data and a client", func() {
			client := &clientOpenAPIStub{
				funcPut: func() (*http.Response, error) {
					return &http.Response{
						StatusCode: expectedReturnCode,
						Body:       ioutil.NopCloser(strings.NewReader("")),
					}, nil
				},
				responsePayload: map[string]interface{}{
					idProperty.Name:     "id",
					stringProperty.Name: "someValue",
				},
			}
			err := r.update(resourceData, client)
			Convey("Then the error returned should be the expected one", func() {
				So(err.Error(), ShouldEqual, "polling mechanism failed after PUT /v1/resource call with response status code (202): error waiting for resource to reach a completion status ([]) [valid pending statuses ([])]: error occurred while retrieving status identifier value from payload for resource 'resourceName' (): could not find any status property. Please make sure the resource schema definition has either one property named 'status' or one property is marked with IsStatusIdentifier set to true")
			})
		})
	})

	Convey("Given a resource factory where getResourcePath returns an error", t, func() {
		r := resourceFactory{
			openAPIResource: &specStubResource{
				funcGetResourcePath: func(parentIDs []string) (s string, e error) {
					return "", errors.New("getResourcePath() failed")
				}},
		}
		Convey("When update is called with resource data and an empty clientOpenAPI", func() {
			err := r.update(&schema.ResourceData{}, &clientOpenAPIStub{})
			Convey("Then the error returned should be the expected one", func() {
				assert.EqualError(t, err, "getResourcePath() failed")
			})
		})
	})
}

func TestDelete(t *testing.T) {
	Convey("Given a resource factory", t, func() {
		var telemetryHandlerResourceNameReceived string
		var telemetryHandlerTFOperationReceived TelemetryResourceOperation
		r, resourceData := testCreateResourceFactoryWithID(t, idProperty)
		Convey("When delete is called with resource data and a client", func() {
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					idProperty.Name: idProperty.Default,
				},
				telemetryHandler: &telemetryHandlerStub{
					submitResourceExecutionMetricsFunc: func(resourceName string, tfOperation TelemetryResourceOperation) {
						telemetryHandlerResourceNameReceived = resourceName
						telemetryHandlerTFOperationReceived = tfOperation
					},
				},
			}
			err := r.delete(resourceData, client)
			Convey("Then the error returned should be nil", func() {
				So(err, ShouldBeNil)
			})
			Convey("Then the expectedValue returned should be true", func() {
				So(client.responsePayload, ShouldNotContainKey, idProperty.Name)
			})
			Convey("And the expected telemetry provider should have been called", func() {
				So(telemetryHandlerResourceNameReceived, ShouldEqual, "resourceName")
				So(telemetryHandlerTFOperationReceived, ShouldEqual, TelemetryResourceOperationDelete)
			})
		})
		Convey("When delete is called with resource data and a client configured to return an error when delete is called", func() {
			deleteError := fmt.Errorf("some error when deleting")
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					idProperty.Name: idProperty.Default,
				},
				error: deleteError,
			}
			err := r.delete(resourceData, client)
			Convey("Then the error returned should not be nil", func() {
				So(err, ShouldNotBeNil)
			})
			Convey("Then the error returned should be the error returned by the client delete operation", func() {
				So(err, ShouldEqual, deleteError)
			})
		})

		Convey("When update is called with resource data and a client returns a non expected http code", func() {
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{},
				returnHTTPCode:  http.StatusInternalServerError,
			}
			err := r.delete(resourceData, client)
			Convey("Then the error returned should NOT be nil", func() {
				So(err, ShouldNotBeNil)
			})
			Convey("And the error returned should be", func() {
				So(err.Error(), ShouldEqual, "[resource='resourceName'] DELETE /v1/resource/id failed: [resource='resourceName'] HTTP Response Status Code 500 not matching expected one [204 200 202] ()")
			})
		})

		Convey("When update is called with resource data and a client returns a 404 status code; hence the resource effectively no longer exists", func() {
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{},
				returnHTTPCode:  http.StatusNotFound,
			}
			err := r.delete(resourceData, client)
			Convey("Then the error returned should NOT be nil", func() {
				So(err, ShouldBeNil)
			})
		})
	})

	Convey("Given a resource factory with no delete operation configured", t, func() {
		specResource := newSpecStubResource("resourceName", "/v1/resource", false, nil)
		r := newResourceFactory(specResource)
		Convey("When delete is called with resource data and a client", func() {
			client := &clientOpenAPIStub{}
			err := r.delete(&schema.ResourceData{}, client)
			Convey("Then the expectedValue returned should be true", func() {
				So(err, ShouldNotBeNil)
			})
			Convey("And resourceData should be populated with the values returned by the API including the ID", func() {
				So(err.Error(), ShouldEqual, "[resource='resourceName'] resource does not support DELETE operation, check the swagger file exposed on '/v1/resource'")
			})
		})
	})

	Convey("Given a resource factory with an empty OpenAPI resource", t, func() {
		r := resourceFactory{}
		Convey("When delete is called with empty data and a empty client", func() {
			client := &clientOpenAPIStub{}
			err := r.delete(nil, client)
			Convey("Then the error returned should be the expected one", func() {
				So(err.Error(), ShouldEqual, "missing openAPI resource configuration")
			})
		})
	})

	Convey("Given a resource factory that has an asynchronous create operation (delete) but the polling operation fails for some reason", t, func() {
		expectedReturnCode := 202
		testSchema := newTestSchema(idProperty, stringProperty)
		resourceData := testSchema.getResourceData(t)
		specResource := newSpecStubResourceWithOperations("resourceName", "/v1/resource", false, testSchema.getSchemaDefinition(), &specResourceOperation{}, &specResourceOperation{}, &specResourceOperation{}, &specResourceOperation{responses: specResponses{expectedReturnCode: &specResponse{isPollingEnabled: true}}})
		r := resourceFactory{
			openAPIResource: specResource,
			defaultTimeout:  time.Duration(0 * time.Second),
		}
		Convey("When create is called with resource data and a client", func() {
			client := &clientOpenAPIStub{
				returnHTTPCode: expectedReturnCode,
				responsePayload: map[string]interface{}{
					idProperty.Name:     "someID",
					stringProperty.Name: "someExtraValueThatProvesResponseDataIsPersisted",
				},
			}
			err := r.delete(resourceData, client)
			Convey("Then the error returned should be the expected one", func() {
				So(err.Error(), ShouldEqual, "polling mechanism failed after DELETE /v1/resource call with response status code (202): error waiting for resource to reach a completion status ([destroyed]) [valid pending statuses ([])]: error on retrieving resource 'resourceName' () when waiting: [resource='resourceName'] HTTP Response Status Code 202 not matching expected one [200] ()")
			})
		})
	})

	Convey("Given a resource factory where getResourcePath returns an error", t, func() {
		r := resourceFactory{
			openAPIResource: &specStubResource{
				funcGetResourcePath: func(parentIDs []string) (s string, e error) {
					return "", errors.New("getResourcePath() failed")
				}},
		}
		Convey("When delete is called with resource data and an empty clientOpenAPI", func() {
			err := r.delete(&schema.ResourceData{}, &clientOpenAPIStub{})
			Convey("Then the error returned should be the expected one", func() {
				assert.EqualError(t, err, "getResourcePath() failed")
			})
		})
	})
}

func TestImporter(t *testing.T) {
	Convey("Given a resource factory configured with a root resource (and the already populated id property value provided by the user)", t, func() {
		var telemetryHandlerResourceNameReceived []string
		var telemetryHandlerTFOperationReceived []TelemetryResourceOperation
		importedIDProperty := idProperty
		r, resourceData := testCreateResourceFactoryWithID(t, importedIDProperty, stringProperty)
		Convey("When importer is called", func() {
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					stringProperty.Name: "someOtherStringValue",
				},
				telemetryHandler: &telemetryHandlerStub{
					submitResourceExecutionMetricsFunc: func(resourceName string, tfOperation TelemetryResourceOperation) {
						telemetryHandlerResourceNameReceived = append(telemetryHandlerResourceNameReceived, resourceName)
						telemetryHandlerTFOperationReceived = append(telemetryHandlerTFOperationReceived, tfOperation)
					},
				},
			}
			resourceImporter := r.importer()
			Convey("Then the resource importer returned should Not be nil", func() {
				So(resourceImporter, ShouldNotBeNil)
			})
			Convey("And when the resourceImporter State method is invoked with data resource and the provider client", func() {
				data, err := resourceImporter.State(resourceData, client)
				Convey("Then the err returned should be nil", func() {
					So(err, ShouldBeNil)
				})
				Convey("And the data list returned should have one item", func() {
					So(len(data), ShouldEqual, 1)
				})
				Convey("And the data returned should contained the expected resource ID", func() {
					So(data[0].Id(), ShouldEqual, idProperty.Default)
				})
				Convey("And the data returned should contained the imported id field with the right value", func() {
					So(data[0].Get(importedIDProperty.Name), ShouldEqual, importedIDProperty.Default)
				})
				Convey("And the data returned should contained the imported string field with the right value returned from the API", func() {
					So(data[0].Get(stringProperty.Name), ShouldEqual, client.responsePayload[stringProperty.Name])
				})
				Convey("And the expected telemetry provider should have been called", func() {
					So(telemetryHandlerResourceNameReceived[0], ShouldEqual, "resourceName")
					So(telemetryHandlerTFOperationReceived[0], ShouldEqual, TelemetryResourceOperationImport)
					So(telemetryHandlerResourceNameReceived[1], ShouldEqual, "resourceName")
					So(telemetryHandlerTFOperationReceived[1], ShouldEqual, TelemetryResourceOperationRead)
				})
			})
		})
	})

	Convey("Given a resource factory", t, func() {
		importedIDProperty := idProperty
		r, resourceData := testCreateResourceFactoryWithID(t, importedIDProperty, stringProperty)
		Convey("When importer is called but the API returns 404", func() {
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					stringProperty.Name: "someOtherStringValue",
				},
				returnHTTPCode: http.StatusNotFound,
			}
			resourceImporter := r.importer()
			Convey("Then the resource importer returned should Not be nil", func() {
				So(resourceImporter, ShouldNotBeNil)
			})
			Convey("And when the resourceImporter State method is invoked with data resource and the provider client", func() {
				data, err := resourceImporter.State(resourceData, client)
				Convey("Then the err returned should mention that the resource does not exists", func() {
					So(err.Error(), ShouldEqual, "[resource='resourceName'] GET /v1/resource/id failed: HTTP Response Status Code 404 - Not Found. Could not find resource instance: ")
				})
				Convey("Then the data should be nil", func() {
					So(data, ShouldBeNil)
				})
			})
		})
	})

	Convey("Given a resource factory configured with a sub-resource (and the already populated id property value provided by the user with the correct format)", t, func() {
		expectedParentID := "32"
		expectedResourceInstanceID := "159"
		resourceParentName := "cdns_v1"
		expectedParentPropertyName := fmt.Sprintf("%s_id", resourceParentName)

		importedIDValue := fmt.Sprintf("%s/%s", expectedParentID, expectedResourceInstanceID)
		importedIDProperty := newStringSchemaDefinitionProperty("id", "", true, true, false, false, false, true, false, false, importedIDValue)
		expectedParentProperty := newStringSchemaDefinitionProperty(expectedParentPropertyName, "", true, true, false, false, false, true, false, false, "")
		r, resourceData := testCreateSubResourceFactory(t, "/v1/cdns/{id}/firewall", []string{resourceParentName}, "cdns_v1", importedIDProperty, stringProperty, expectedParentProperty)

		Convey("When importer is called", func() {
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					stringProperty.Name: "someOtherStringValue",
				},
			}
			resourceImporter := r.importer()
			Convey("Then the resource importer returned should Not be nil", func() {
				So(resourceImporter, ShouldNotBeNil)
			})
			Convey("And when the resourceImporter State method is invoked with the provider client and resource data for one item", func() {
				data, err := resourceImporter.State(resourceData, client)
				Convey("Then the err returned should be nil", func() {
					So(err, ShouldBeNil)
				})
				Convey("And the data list returned should have one item", func() {
					So(len(data), ShouldEqual, 1)
				})
				Convey("And the data returned should contain the parent id field with the right value", func() {
					So(data[0].Get(expectedParentPropertyName), ShouldEqual, expectedParentID)
				})
				Convey("And the data returned should contain the expected resource ID", func() {
					So(data[0].Id(), ShouldEqual, expectedResourceInstanceID)
				})
				Convey("And the data returned should contain the imported string field with the right value returned from the API", func() {
					So(data[0].Get(stringProperty.Name), ShouldEqual, client.responsePayload[stringProperty.Name])
				})
			})
		})
	})

	Convey("Given a resource factory configured with a sub-resource (and the already populated id property value provided by the user with incorrect format)", t, func() {
		resourceParentName := "cdns_v1"
		expectedParentPropertyName := fmt.Sprintf("%s_id", resourceParentName)

		importedIDValue := "someStringThatDoesNotMatchTheExpectedSubResourceIDFormat"
		importedIDProperty := newStringSchemaDefinitionProperty("id", "", true, true, false, false, false, true, false, false, importedIDValue)
		expectedParentProperty := newStringSchemaDefinitionProperty(expectedParentPropertyName, "", true, true, false, false, false, true, false, false, "")
		r, resourceData := testCreateSubResourceFactory(t, "/v1/cdns/{id}/firewall", []string{resourceParentName}, "cdns_v1", importedIDProperty, stringProperty, expectedParentProperty)

		Convey("When importer is called", func() {
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					stringProperty.Name: "someOtherStringValue",
				},
			}
			resourceImporter := r.importer()
			Convey("Then the resource importer returned should Not be nil", func() {
				So(resourceImporter, ShouldNotBeNil)
			})
			Convey("And when the resourceImporter State method is invoked with data resource and the provider client", func() {
				_, err := resourceImporter.State(resourceData, client)
				Convey("Then the err returned should be the expected one", func() {
					So(err.Error(), ShouldEqual, "can not import a subresource without providing all the parent IDs (1) and the instance ID")
				})
			})
		})
	})

	Convey("Given a resource factory configured with a sub-resource (and the already populated id property value contains more IDs than the resource number of parent properties)", t, func() {
		resourceParentName := "cdns_v1"
		expectedParentPropertyName := fmt.Sprintf("%s_id", resourceParentName)

		importedIDValue := "/extraID/1234/23564"
		importedIDProperty := newStringSchemaDefinitionProperty("id", "", true, true, false, false, false, true, false, false, importedIDValue)
		expectedParentProperty := newStringSchemaDefinitionProperty(expectedParentPropertyName, "", true, true, false, false, false, true, false, false, "")
		r, resourceData := testCreateSubResourceFactory(t, "/v1/cdns/{id}/firewall", []string{resourceParentName}, "cdns_v1", importedIDProperty, stringProperty, expectedParentProperty)

		Convey("When importer is called", func() {
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					stringProperty.Name: "someOtherStringValue",
				},
			}
			resourceImporter := r.importer()
			Convey("Then the resource importer returned should Not be nil", func() {
				So(resourceImporter, ShouldNotBeNil)
			})
			Convey("And when the resourceImporter State method is invoked with data resource and the provider client", func() {
				_, err := resourceImporter.State(resourceData, client)
				Convey("Then the err returned should be the expected one", func() {
					So(err.Error(), ShouldEqual, "the number of parent IDs provided 3 is greater than the expected number of parent IDs 1")
				})
			})
		})
	})

	Convey("Given a resource factory configured with a sub-resource (and the already populated id property value contains less IDs than the resource number of parent properties)", t, func() {
		resourceParentParentName := "cdns_v1"
		resourceParentName := "cdns_v1_firewalls_v1"
		importedIDValue := "1234/5647" // missing one of the parent ids
		importedIDProperty := newStringSchemaDefinitionProperty("id", "", true, true, false, false, false, true, false, false, importedIDValue)
		expectedParentProperty := newStringSchemaDefinitionProperty("cdns_v1_firewalls_v1", "", true, true, false, false, false, true, false, false, "")
		r, resourceData := testCreateSubResourceFactory(t, "/v1/cdns/{id}/firewalls/{id}/rules", []string{resourceParentParentName, resourceParentName}, "cdns_v1_firewalls_v1", importedIDProperty, stringProperty, expectedParentProperty)

		Convey("When importer is called", func() {
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					stringProperty.Name: "someOtherStringValue",
				},
			}
			resourceImporter := r.importer()
			Convey("Then the resource importer returned should Not be nil", func() {
				So(resourceImporter, ShouldNotBeNil)
			})
			Convey("And when the resourceImporter State method is invoked with data resource and the provider client", func() {
				_, err := resourceImporter.State(resourceData, client)
				Convey("Then the err returned should be the expected one", func() {
					So(err.Error(), ShouldEqual, "can not import a subresource without all the parent ids, expected 2 and got 1 parent IDs")
				})
			})
		})
	})

	Convey("Given a resource factory configured with a sub-resource missing the parent resource property in the schema", t, func() {
		importedIDValue := "1234/5678"
		importedIDProperty := newStringSchemaDefinitionProperty("id", "", true, true, false, false, false, true, false, false, importedIDValue)
		r, resourceData := testCreateSubResourceFactory(t, "/v1/cdns/{id}/firewall", []string{"cdns_v1"}, "cdns_v1", importedIDProperty, stringProperty)

		Convey("When importer is called", func() {
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					stringProperty.Name: "someOtherStringValue",
				},
			}
			resourceImporter := r.importer()
			Convey("Then the resource importer returned should Not be nil", func() {
				So(resourceImporter, ShouldNotBeNil)
			})
			Convey("And when the resourceImporter State method is invoked with data resource and the provider client", func() {
				_, err := resourceImporter.State(resourceData, client)
				Convey("Then the err returned should be the expected one", func() {
					So(err.Error(), ShouldEqual, `Invalid address to set: []string{"cdns_v1_id"}`)
				})
			})
		})
	})

	Convey("Given a resource factory with an empty OpenAPI resource", t, func() {
		r := resourceFactory{}
		Convey("When import is called", func() {
			resourceImporter := r.importer()
			Convey("Then resourceImporter not be nil", func() {
				So(resourceImporter, ShouldNotBeNil)
				Convey("And when the resourceImporter State method is invoked with data resource and the provider client", func() {
					data, err := resourceImporter.State(&schema.ResourceData{}, &clientOpenAPIStub{})
					Convey("Then the error returned should be the expected one and data should be nil", func() {
						So(err.Error(), ShouldEqual, "missing openAPI resource configuration")
						So(data, ShouldBeNil)
					})
				})
			})
		})
	})

	Convey("Given a resource factory where getResourcePath returns an error", t, func() {
		r := resourceFactory{
			openAPIResource: &specStubResource{
				funcGetResourcePath: func(parentIDs []string) (s string, e error) {
					return "", errors.New("getResourcePath() failed")
				}},
		}
		Convey("When import is called", func() {
			resourceImporter := r.importer()
			Convey("Then resourceImporter not be nil", func() {
				So(resourceImporter, ShouldNotBeNil)
				Convey("And when the resourceImporter State method is invoked with data resource and the provider client", func() {
					resourceData := &schema.ResourceData{}
					data, err := resourceImporter.State(resourceData, &clientOpenAPIStub{})
					Convey("Then the error returned should be the expected one and data should be nil", func() {
						So(err.Error(), ShouldEqual, "getResourcePath() failed")
						So(data, ShouldBeNil)
					})
				})
			})
		})
	})
}

func TestHandlePollingIfConfigured(t *testing.T) {
	Convey("Given a resource factory configured with a resource which has a schema definition containing a status property", t, func() {
		r, resourceData := testCreateResourceFactoryWithID(t, idProperty, stringProperty, statusProperty)
		Convey("When handlePollingIfConfigured is called with an operation that has a response defined for the API response status code passed in and polling is enabled AND the API returns a status that matches the target", func() {
			targetState := "deployed"
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					idProperty.Name:     idProperty.Default,
					stringProperty.Name: stringProperty.Default,
					statusProperty.Name: targetState,
				},
				returnHTTPCode: http.StatusOK,
			}

			responsePayload := map[string]interface{}{}

			responseStatusCode := http.StatusAccepted
			operation := &specResourceOperation{
				responses: map[int]*specResponse{
					responseStatusCode: {
						isPollingEnabled:    true,
						pollPendingStatuses: []string{"pending"},
						pollTargetStatuses:  []string{targetState},
					},
				},
			}
			err := r.handlePollingIfConfigured(&responsePayload, resourceData, client, operation, responseStatusCode, schema.TimeoutCreate)
			Convey("Then the err returned should be nil", func() {
				So(err, ShouldBeNil)
			})
			Convey("And the remote data should be the payload returned by the API", func() {
				So(responsePayload[idProperty.Name], ShouldEqual, client.responsePayload[idProperty.Name])
				So(responsePayload[stringProperty.Name], ShouldEqual, client.responsePayload[stringProperty.Name])
				So(responsePayload[statusProperty.Name], ShouldEqual, client.responsePayload[statusProperty.Name])
			})
		})

		Convey("When handlePollingIfConfigured is called with an operation that has a response defined for the API response status code passed in and polling is enabled AND the responsePayload is nil (meaning we are handling a DELETE operation)", func() {
			targetState := "deployed"
			client := &clientOpenAPIStub{
				returnHTTPCode: http.StatusNotFound,
			}
			responsePayload := map[string]interface{}{}

			responseStatusCode := http.StatusAccepted
			operation := &specResourceOperation{
				responses: map[int]*specResponse{
					responseStatusCode: {
						isPollingEnabled:    true,
						pollPendingStatuses: []string{"pending"},
						pollTargetStatuses:  []string{targetState},
					},
				},
			}
			err := r.handlePollingIfConfigured(nil, resourceData, client, operation, responseStatusCode, schema.TimeoutCreate)
			Convey("Then the err returned should be nil", func() {
				So(err, ShouldBeNil)
			})
			Convey("And the remote data should be the payload returned by the API", func() {
				So(responsePayload[idProperty.Name], ShouldEqual, client.responsePayload[idProperty.Name])
				So(responsePayload[stringProperty.Name], ShouldEqual, client.responsePayload[stringProperty.Name])
				So(responsePayload[statusProperty.Name], ShouldEqual, client.responsePayload[statusProperty.Name])
			})
		})

		Convey("When handlePollingIfConfigured is called with a response status code that DOES NOT any of the operation's response definitions", func() {
			client := &clientOpenAPIStub{}
			responseStatusCode := http.StatusAccepted
			operation := &specResourceOperation{
				responses: map[int]*specResponse{},
			}
			err := r.handlePollingIfConfigured(nil, resourceData, client, operation, responseStatusCode, schema.TimeoutCreate)
			Convey("Then the err  should be nil", func() {
				So(err, ShouldBeNil)
			})
		})

		Convey("When handlePollingIfConfigured is called with a response status code that DOES math one of the operation responses BUT polling is not enabled for that response", func() {
			client := &clientOpenAPIStub{}
			responseStatusCode := http.StatusAccepted
			operation := &specResourceOperation{
				responses: map[int]*specResponse{
					responseStatusCode: {
						isPollingEnabled:    false,
						pollPendingStatuses: []string{"pending"},
						pollTargetStatuses:  []string{"deployed"},
					},
				},
			}
			err := r.handlePollingIfConfigured(nil, resourceData, client, operation, responseStatusCode, schema.TimeoutCreate)
			Convey("Then the err returned should be nil", func() {
				So(err, ShouldBeNil)
			})
		})
	})

	Convey("Given a resource factory that has an asynchronous create operation (post) but the polling operation fails for some reason", t, func() {
		expectedReturnCode := http.StatusAccepted
		r, resourceData := testCreateResourceFactoryWithID(t, idProperty, stringProperty, statusProperty)
		Convey("When create is called with resource data and a client", func() {
			operation := &specResourceOperation{
				responses: map[int]*specResponse{
					expectedReturnCode: {
						isPollingEnabled:    true,
						pollPendingStatuses: []string{"pending"},
						pollTargetStatuses:  []string{"deployed"},
					},
				},
			}
			client := &clientOpenAPIStub{
				returnHTTPCode: expectedReturnCode,
				responsePayload: map[string]interface{}{
					idProperty.Name:     "someID",
					stringProperty.Name: "someExtraValueThatProvesResponseDataIsPersisted",
				},
				error: fmt.Errorf("some error"),
			}
			err := r.handlePollingIfConfigured(nil, resourceData, client, operation, expectedReturnCode, schema.TimeoutCreate)
			Convey("Then the error returned should be the expected one", func() {
				So(err.Error(), ShouldEqual, "error waiting for resource to reach a completion status ([destroyed]) [valid pending statuses ([pending])]: error on retrieving resource 'resourceName' (id) when waiting: some error")
			})
		})
	})

}

func TestResourceStateRefreshFunc(t *testing.T) {
	Convey("Given a resource factory configured with a resource which has a schema definition containing a status property", t, func() {
		r, resourceData := testCreateResourceFactoryWithID(t, idProperty, stringProperty, statusProperty)
		Convey("When resourceStateRefreshFunc is called with an update resource data and an open api client and the returned function (stateRefreshFunc) is invoked", func() {
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					idProperty.Name:     idProperty.Default,
					stringProperty.Name: stringProperty.Default,
					statusProperty.Name: statusProperty.Default,
				},
			}
			stateRefreshFunc := r.resourceStateRefreshFunc(resourceData, client)
			remoteData, newStatus, err := stateRefreshFunc()
			Convey("Then the err returned should be nil", func() {
				So(err, ShouldBeNil)
			})
			Convey("And the new status should match the one returned by the API", func() {
				So(newStatus, ShouldEqual, client.responsePayload[statusProperty.Name])
			})
			Convey("And the remote data should be the payload returned by the API", func() {
				So(remoteData.(map[string]interface{})[idProperty.Name], ShouldEqual, client.responsePayload[idProperty.Name])
				So(remoteData.(map[string]interface{})[stringProperty.Name], ShouldEqual, client.responsePayload[stringProperty.Name])
				So(remoteData.(map[string]interface{})[statusProperty.Name], ShouldEqual, client.responsePayload[statusProperty.Name])
			})

		})
		Convey("When resourceStateRefreshFunc is called with an update resource data and an open api client that returns 404 not found", func() {
			client := &clientOpenAPIStub{
				returnHTTPCode: http.StatusNotFound,
			}
			stateRefreshFunc := r.resourceStateRefreshFunc(resourceData, client)
			_, newStatus, err := stateRefreshFunc()
			Convey("Then the err returned should be nil", func() {
				So(err, ShouldBeNil)
			})
			Convey("And the new status should be the internal hardcoded status 'destroyed' as a response with 404 status code is not expected to have a body", func() {
				So(newStatus, ShouldEqual, defaultDestroyStatus)
			})
		})

		Convey("When resourceStateRefreshFunc is called with an update resource data and an open api client that returns an error", func() {
			expectedError := "some error"
			client := &clientOpenAPIStub{
				error: errors.New(expectedError),
			}
			stateRefreshFunc := r.resourceStateRefreshFunc(resourceData, client)
			remoteData, newStatus, err := stateRefreshFunc()
			Convey("Then the err returned should not be nil", func() {
				So(err, ShouldNotBeNil)
			})
			Convey("And the error message should be the expected one", func() {
				So(err.Error(), ShouldEqual, fmt.Sprintf("error on retrieving resource 'resourceName' (id) when waiting: %s", expectedError))
			})
			Convey("And the remoteData should be empty", func() {
				So(remoteData, ShouldBeNil)
			})
			Convey("And the new status should be empty", func() {
				So(newStatus, ShouldBeEmpty)
			})
		})
	})

	Convey("Given a resource factory configured with a resource which has a schema definition missing a status property", t, func() {
		r, resourceData := testCreateResourceFactoryWithID(t, idProperty, stringProperty)
		Convey("When resourceStateRefreshFunc is called with an update resource data and an open api client that returns an error", func() {
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					idProperty.Name:     idProperty.Default,
					stringProperty.Name: stringProperty.Default,
				},
			}
			stateRefreshFunc := r.resourceStateRefreshFunc(resourceData, client)
			remoteData, newStatus, err := stateRefreshFunc()
			Convey("Then the err returned should not be nil", func() {
				So(err, ShouldNotBeNil)
			})
			Convey("And the error message should be the expected one", func() {
				So(err.Error(), ShouldEqual, "error occurred while retrieving status identifier value from payload for resource 'resourceName' (id): could not find any status property. Please make sure the resource schema definition has either one property named 'status' or one property is marked with IsStatusIdentifier set to true")
			})
			Convey("And the remoteData should be empty", func() {
				So(remoteData, ShouldBeNil)
			})
			Convey("And the new status should be empty", func() {
				So(newStatus, ShouldBeEmpty)
			})
		})
	})

	Convey("Given a resource factory configured with a resource which has a schema definition with a status property but the responsePayload is missing the status property", t, func() {
		r, resourceData := testCreateResourceFactoryWithID(t, idProperty, stringProperty, statusProperty)
		Convey("When resourceStateRefreshFunc is called with an update resource data and an open api client that returns an error", func() {
			client := &clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					idProperty.Name:     idProperty.Default,
					stringProperty.Name: stringProperty.Default,
				},
			}
			stateRefreshFunc := r.resourceStateRefreshFunc(resourceData, client)
			remoteData, newStatus, err := stateRefreshFunc()
			Convey("Then the err returned should not be nil", func() {
				So(err, ShouldNotBeNil)
			})
			Convey("And the error message should be the expected one", func() {
				So(err.Error(), ShouldEqual, "error occurred while retrieving status identifier value from payload for resource 'resourceName' (id): payload does not match resouce schema, could not find the status field: [status]")
			})
			Convey("And the remoteData should be empty", func() {
				So(remoteData, ShouldBeNil)
			})
			Convey("And the new status should be empty", func() {
				So(newStatus, ShouldBeEmpty)
			})
		})
	})
}

func TestCheckImmutableFields(t *testing.T) {

	testCases := []struct {
		name          string
		inputProps    []*SpecSchemaDefinitionProperty
		client        clientOpenAPIStub
		assertions    func(*schema.ResourceData)
		expectedError error
	}{
		{
			name: "mutable string property is updated",
			inputProps: []*SpecSchemaDefinitionProperty{
				{
					Name:      "mutable_prop",
					Type:      TypeString,
					Immutable: false,
					Default:   4,
				}, // this pretends the property value in the state file has been updated
			},
			client: clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					"mutable_prop": "originalPropertyValue",
				},
			},
			expectedError: nil,
		},
		{
			name: "immutable string property is updated",
			inputProps: []*SpecSchemaDefinitionProperty{
				{
					Name:      "immutable_prop",
					Type:      TypeString,
					Immutable: true,
					Default:   "updatedImmutableValue",
				}, // this pretends the property value in the state file has been updated
			},
			client: clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					"immutable_prop": "originalImmutablePropertyValue",
				},
			},
			assertions: func(resourceData *schema.ResourceData) {
				assert.Equal(t, "originalImmutablePropertyValue", resourceData.Get("immutable_prop"))
			},
			expectedError: errors.New("validation for immutable properties failed: user attempted to update an immutable property ('immutable_prop'): [user input: updatedImmutableValue; actual: originalImmutablePropertyValue]. Update operation was aborted; no updates were performed"),
		},
		{
			name: "immutable int property is updated",
			inputProps: []*SpecSchemaDefinitionProperty{
				{
					Name:      "immutable_prop",
					Type:      TypeInt,
					Immutable: true,
					Default:   4,
				},
			},
			client: clientOpenAPIStub{
				responsePayload: getMapFromJSON(t, `{"immutable_prop": 6}`),
			},
			assertions: func(resourceData *schema.ResourceData) {
				assert.Equal(t, 6, resourceData.Get("immutable_prop"))
			},
			expectedError: errors.New("validation for immutable properties failed: user attempted to update an immutable integer property ('immutable_prop'): [user input: 4; actual: 6]. Update operation was aborted; no updates were performed"),
		},
		{
			name: "immutable int property has not changed",
			inputProps: []*SpecSchemaDefinitionProperty{
				{
					Name:      "immutable_prop",
					Type:      TypeInt,
					Immutable: true,
					Default:   4,
				},
			},
			client: clientOpenAPIStub{
				responsePayload: getMapFromJSON(t, `{"immutable_prop": 4}`),
			},
			assertions: func(resourceData *schema.ResourceData) {
				assert.Equal(t, 4, resourceData.Get("immutable_prop"))
			},
			expectedError: nil,
		},
		{
			name: "immutable float property is updated",
			inputProps: []*SpecSchemaDefinitionProperty{
				{
					Name:      "immutable_prop",
					Type:      TypeFloat,
					Immutable: true,
					Default:   4.5,
				},
			},
			client: clientOpenAPIStub{
				responsePayload: getMapFromJSON(t, `{"immutable_prop": 3.8}`),
			},
			assertions: func(resourceData *schema.ResourceData) {
				assert.Equal(t, 3.8, resourceData.Get("immutable_prop"))
			},
			expectedError: errors.New("validation for immutable properties failed: user attempted to update an immutable float property ('immutable_prop'): [user input: %!s(float64=4.5); actual: %!s(float64=3.8)]. Update operation was aborted; no updates were performed"),
		},
		{
			name: "immutable float property has not changed",
			inputProps: []*SpecSchemaDefinitionProperty{
				{
					Name:      "immutable_prop",
					Type:      TypeFloat,
					Immutable: true,
					Default:   4.5,
				},
			},
			client: clientOpenAPIStub{
				responsePayload: getMapFromJSON(t, `{"immutable_prop": 4.5}`),
			},
			assertions: func(resourceData *schema.ResourceData) {
				assert.Equal(t, 4.5, resourceData.Get("immutable_prop"))
			},
			expectedError: nil,
		},
		{
			name: "immutable bool property is updated",
			inputProps: []*SpecSchemaDefinitionProperty{
				{
					Name:      "immutable_prop",
					Type:      TypeBool,
					Immutable: true,
					Default:   true,
				},
			},
			client: clientOpenAPIStub{
				responsePayload: getMapFromJSON(t, `{"immutable_prop": false}`),
			},
			assertions: func(resourceData *schema.ResourceData) {
				assert.Equal(t, false, resourceData.Get("immutable_prop"))
			},
			expectedError: errors.New("validation for immutable properties failed: user attempted to update an immutable property ('immutable_prop'): [user input: %!s(bool=true); actual: %!s(bool=false)]. Update operation was aborted; no updates were performed"),
		},
		{
			name: "immutable list property is updated",
			inputProps: []*SpecSchemaDefinitionProperty{
				{
					Name:           "immutable_prop",
					Type:           TypeList,
					ArrayItemsType: TypeString,
					Immutable:      true,
					Default:        []interface{}{"value1Updated", "value2Updated"},
				},
			},
			client: clientOpenAPIStub{
				responsePayload: getMapFromJSON(t, `{"immutable_prop": ["value1","value2"]}`),
			},
			assertions: func(resourceData *schema.ResourceData) {
				assert.Equal(t, []interface{}{"value1", "value2"}, resourceData.Get("immutable_prop"))
			},
			expectedError: errors.New("validation for immutable properties failed: user attempted to update an immutable list property ('immutable_prop') element: [user input: [value1Updated value2Updated]; actual: [value1 value2]]. Update operation was aborted; no updates were performed"),
		},
		{
			name: "mutable list property is updated",
			inputProps: []*SpecSchemaDefinitionProperty{
				{
					Name:           "mutable_prop",
					Type:           TypeList,
					ArrayItemsType: TypeString,
					Immutable:      false,
					Default:        []interface{}{"value1Updated", "value2Updated", "newValue"},
				},
			},
			client: clientOpenAPIStub{
				responsePayload: getMapFromJSON(t, `{"mutable_prop": ["value1","value2"]}`),
			},
			assertions: func(resourceData *schema.ResourceData) {
				assert.Equal(t, []interface{}{"value1Updated", "value2Updated", "newValue"}, resourceData.Get("mutable_prop"))
			},
			expectedError: nil,
		},
		{
			name: "immutable list size is updated",
			inputProps: []*SpecSchemaDefinitionProperty{
				{
					Name:           "immutable_prop",
					Type:           TypeList,
					ArrayItemsType: TypeString,
					Immutable:      true,
					Default:        []interface{}{"value1Updated", "value2Updated", "value3Updated"},
				},
			},
			client: clientOpenAPIStub{
				responsePayload: getMapFromJSON(t, `{"immutable_prop": ["value1","value2"]}`),
			},
			assertions: func(resourceData *schema.ResourceData) {
				assert.Equal(t, []interface{}{"value1", "value2"}, resourceData.Get("immutable_prop"))
			},
			expectedError: errors.New("validation for immutable properties failed: user attempted to update an immutable list property ('immutable_prop') size: [user input list size: 3; actual list size: 2]. Update operation was aborted; no updates were performed"),
		},
		{
			name: "immutable object property is updated",
			inputProps: []*SpecSchemaDefinitionProperty{
				{
					Name:      "immutable_prop",
					Type:      TypeObject,
					Immutable: true,
					SpecSchemaDefinition: &SpecSchemaDefinition{
						Properties: SpecSchemaDefinitionProperties{
							readOnlyProperty,
							newIntSchemaDefinitionPropertyWithDefaults("origin_port", "", true, false, 80),
							newStringSchemaDefinitionPropertyWithDefaults("protocol", "", true, false, "http"),
						},
					},
					Default: map[string]interface{}{
						readOnlyProperty.Name: readOnlyProperty.Default,
						"origin_port":         80,
						"protocol":            "http",
					},
				},
			},
			client: clientOpenAPIStub{
				responsePayload: getMapFromJSON(t, `{"immutable_prop": {"read_only_property":"some_value","origin_port":443,"protocol":"https"}}`),
			},
			assertions: func(resourceData *schema.ResourceData) {
				assert.Equal(t, map[string]interface{}{"origin_port": "443", "protocol": "https", "read_only_property": "some_value"}, resourceData.Get("immutable_prop"))
			},
			expectedError: errors.New("validation for immutable properties failed: user attempted to update an immutable object ('immutable_prop') property ('origin_port'): [user input: map[origin_port:%!s(int64=80) protocol:http]; actual: map[origin_port:%!s(float64=443) protocol:https read_only_property:some_value]]. Update operation was aborted; no updates were performed"),
		},
		{
			name: "mutable object properties are updated",
			inputProps: []*SpecSchemaDefinitionProperty{
				{
					Name:      "immutable_prop",
					Type:      TypeObject,
					Immutable: false,
					SpecSchemaDefinition: &SpecSchemaDefinition{
						Properties: SpecSchemaDefinitionProperties{
							readOnlyProperty,
							newIntSchemaDefinitionPropertyWithDefaults("origin_port", "", true, false, 80),
							newStringSchemaDefinitionPropertyWithDefaults("protocol", "", true, false, "http"),
						},
					},
					Default: map[string]interface{}{
						readOnlyProperty.Name: readOnlyProperty.Default,
						"origin_port":         80,
						"protocol":            "http",
					},
				},
			},
			client: clientOpenAPIStub{
				responsePayload: getMapFromJSON(t, `{"immutable_prop": {"read_only_property":"some_value","origin_port":443,"protocol":"https"}}`),
			},
			assertions: func(resourceData *schema.ResourceData) {
				assert.Equal(t, map[string]interface{}{"origin_port": "443", "protocol": "https", "read_only_property": "some_value"}, resourceData.Get("immutable_prop"))
			},
			expectedError: nil,
		},
		{
			name: "immutable property inside a mutable object is updated",
			inputProps: []*SpecSchemaDefinitionProperty{
				{
					Name:      "mutable_prop",
					Type:      TypeObject,
					Immutable: false, // the object in this case is mutable; however some props are immutable
					SpecSchemaDefinition: &SpecSchemaDefinition{
						Properties: SpecSchemaDefinitionProperties{
							immutableProperty,
							newIntSchemaDefinitionPropertyWithDefaults("origin_port", "", true, false, 80),
							newStringSchemaDefinitionPropertyWithDefaults("protocol", "", true, false, "http"),
						},
					},
					Default: map[string]interface{}{
						immutableProperty.Name: immutableProperty.Default,
						"origin_port":          80,
						"protocol":             "http",
					},
				},
			},
			client: clientOpenAPIStub{
				responsePayload: getMapFromJSON(t, `{"mutable_prop": {"string_immutable_property":"some_value","origin_port":443,"protocol":"https"}}`),
			},
			assertions: func(resourceData *schema.ResourceData) {
				assert.Equal(t, map[string]interface{}{"origin_port": "443", "protocol": "https", "string_immutable_property": "some_value"}, resourceData.Get("mutable_prop"))
			},
			expectedError: errors.New("validation for immutable properties failed: user attempted to update an immutable object ('mutable_prop') property ('string_immutable_property'): [user input: map[origin_port:%!s(int64=80) protocol:http string_immutable_property:updatedImmutableValue]; actual: map[origin_port:%!s(float64=443) protocol:https string_immutable_property:some_value]]. Update operation was aborted; no updates were performed"),
		},
		{
			name: "immutable object with nested object property is updated",
			inputProps: []*SpecSchemaDefinitionProperty{
				{
					Name:      "immutable_prop",
					Type:      TypeObject,
					Immutable: true,
					SpecSchemaDefinition: &SpecSchemaDefinition{
						Properties: SpecSchemaDefinitionProperties{
							newObjectSchemaDefinitionPropertyWithDefaults("object_property", "", true, false, false, map[string]interface{}{
								"some_prop": "someValue",
							}, &SpecSchemaDefinition{
								Properties: SpecSchemaDefinitionProperties{
									newStringSchemaDefinitionProperty("some_prop", "", true, false, false, false, false, true, false, false, "someValue"),
								},
							}),
						},
					},
					Default: []interface{}{
						map[string]interface{}{
							"object_property": map[string]interface{}{
								"some_prop": "someUpdatedValue",
							},
						},
					},
				},
			},
			client: clientOpenAPIStub{
				responsePayload: getMapFromJSON(t, `{"immutable_prop": {"object_property": {"some_prop":"someValue"}}}`),
			},
			assertions: func(resourceData *schema.ResourceData) {
				assert.Equal(t, []interface{}{map[string]interface{}{"object_property": map[string]interface{}{"some_prop": "someValue"}}}, resourceData.Get("immutable_prop"))
			},
			expectedError: errors.New("validation for immutable properties failed: user attempted to update an immutable object ('immutable_prop') property ('object_property'): [user input: map[object_property:map[some_prop:someUpdatedValue]]; actual: map[object_property:map[some_prop:someValue]]]. Update operation was aborted; no updates were performed"),
		},
		{
			name: "immutable list of objects is updated",
			inputProps: []*SpecSchemaDefinitionProperty{
				{
					Name:           "immutable_prop",
					Type:           TypeList,
					ArrayItemsType: TypeObject,
					Immutable:      true,
					SpecSchemaDefinition: &SpecSchemaDefinition{
						Properties: SpecSchemaDefinitionProperties{
							newIntSchemaDefinitionPropertyWithDefaults("origin_port", "", true, false, 80),
							newStringSchemaDefinitionPropertyWithDefaults("protocol", "", true, false, "http"),
						},
					},
					Default: []interface{}{
						map[string]interface{}{
							"origin_port": 80,
							"protocol":    "http",
						},
					},
				},
			},
			client: clientOpenAPIStub{
				responsePayload: getMapFromJSON(t, `{"immutable_prop": [{"origin_port":443, "protocol":"https"}]}`),
			},
			assertions: func(resourceData *schema.ResourceData) {
				assert.Equal(t, []interface{}{map[string]interface{}{"origin_port": 443, "protocol": "https"}}, resourceData.Get("immutable_prop"))
			},
			expectedError: errors.New("validation for immutable properties failed: user attempted to update an immutable list of objects ('immutable_prop'): [user input: [map[origin_port:%!s(int=80) protocol:http]]; actual: [map[origin_port:%!s(float64=443) protocol:https]]]. Update operation was aborted; no updates were performed"),
		},
		{
			name: "mutable list of objects where some properties are immutable and values are not updated",
			inputProps: []*SpecSchemaDefinitionProperty{
				{
					Name:           "immutable_prop",
					Type:           TypeList,
					ArrayItemsType: TypeObject,
					Immutable:      false,
					SpecSchemaDefinition: &SpecSchemaDefinition{
						Properties: SpecSchemaDefinitionProperties{
							&SpecSchemaDefinitionProperty{
								Name:      "origin_port",
								Type:      TypeInt,
								Required:  true,
								ReadOnly:  false,
								Immutable: true,
								Default:   80,
							},
							&SpecSchemaDefinitionProperty{
								Name:      "protocol",
								Type:      TypeString,
								Required:  true,
								ReadOnly:  false,
								Immutable: true,
								Default:   "http",
							},
							&SpecSchemaDefinitionProperty{
								Name:      "float_prop",
								Type:      TypeFloat,
								Required:  true,
								ReadOnly:  false,
								Immutable: true,
								Default:   99.99,
							},
							&SpecSchemaDefinitionProperty{
								Name:      "enabled",
								Type:      TypeBool,
								Required:  true,
								ReadOnly:  false,
								Immutable: true,
								Default:   true,
							},
						},
					},
					Default: []interface{}{
						map[string]interface{}{
							"origin_port": 80,
							"protocol":    "http",
							"float_prop":  99.99,
							"enabled":     true,
						},
					},
				},
			},
			client: clientOpenAPIStub{
				responsePayload: getMapFromJSON(t, `{"immutable_prop": [{"origin_port":80, "protocol":"http", "float_prop":99.99,"enabled":true}]}`),
			},
			assertions: func(resourceData *schema.ResourceData) {
				assert.Equal(t, []interface{}{map[string]interface{}{"origin_port": 80, "protocol": "http", "float_prop": 99.99, "enabled": true}}, resourceData.Get("immutable_prop"))
			},
			expectedError: nil,
		},
		{
			name:       "client returns an error",
			inputProps: []*SpecSchemaDefinitionProperty{},
			client: clientOpenAPIStub{
				error: errors.New("some error"),
			},
			assertions:    func(resourceData *schema.ResourceData) {},
			expectedError: errors.New("some error"),
		},
		{
			name: "immutable property is updated",
			inputProps: []*SpecSchemaDefinitionProperty{
				{
					Name:      "immutable_prop",
					Type:      TypeString,
					Immutable: true,
					Default:   "updatedImmutableValue",
				},
			},
			client: clientOpenAPIStub{
				responsePayload: map[string]interface{}{
					"immutable_prop": "originalImmutablePropertyValue",
					"unknown_prop":   "some value",
				},
			},
			assertions:    func(resourceData *schema.ResourceData) {},
			expectedError: errors.New("validation for immutable properties failed: user attempted to update an immutable property ('immutable_prop'): [user input: updatedImmutableValue; actual: originalImmutablePropertyValue]. Update operation was aborted; no updates were performed"),
		},
	}

	for _, tc := range testCases {
		r, resourceData := testCreateResourceFactory(t, tc.inputProps...)
		err := r.checkImmutableFields(resourceData, &tc.client)
		if tc.expectedError == nil {
			assert.NoError(t, err, tc.name)
		} else {
			assert.Equal(t, tc.expectedError, err, tc.name)
			tc.assertions(resourceData)
		}
	}
}

func getMapFromJSON(t *testing.T, input string) map[string]interface{} {
	var m map[string]interface{}
	err := json.Unmarshal([]byte(input), &m)
	if err != nil {
		log.Fatal(err)
	}
	return m
}

func TestCreatePayloadFromLocalStateData(t *testing.T) {
	idProperty := newStringSchemaDefinitionProperty("id", "", false, true, false, false, false, true, false, false, "id")
	testCases := []struct {
		name            string
		inputProps      []*SpecSchemaDefinitionProperty
		expectedPayload map[string]interface{}
	}{
		{
			name: "id and computed properties are not part of the payload",
			inputProps: []*SpecSchemaDefinitionProperty{
				idProperty,
				computedProperty,
			},
			expectedPayload: map[string]interface{}{},
		},
		{
			name: "id and property marked as preferred identifier is not part of the payload",
			inputProps: []*SpecSchemaDefinitionProperty{
				idProperty,
				newStringSchemaDefinitionProperty("someOtherID", "", false, true, false, false, false, true, true, false, "someOtherIDValue"),
			},
			expectedPayload: map[string]interface{}{},
		},
		{
			name: "parent properties are not part of the payload",
			inputProps: []*SpecSchemaDefinitionProperty{
				newParentStringSchemaDefinitionPropertyWithDefaults("parentProperty", "", true, false, "http"),
				stringProperty,
			},
			expectedPayload: map[string]interface{}{
				stringProperty.GetTerraformCompliantPropertyName(): stringProperty.Default,
			},
		},
		{
			// - Representation of resourceData configuration containing an object
			// {
			//	 string_property = "updatedValue"
			//	 object_property = {
			//		origin_port = 80
			//		protocol = "http"
			//	 }
			// }
			name: "properties within objects that are computed should not be in the payload",
			inputProps: []*SpecSchemaDefinitionProperty{
				stringProperty,
				newObjectSchemaDefinitionPropertyWithDefaults("object_property", "", true, false, false, map[string]interface{}{
					"origin_port": 80,
					"protocol":    "http",
					computedProperty.GetTerraformCompliantPropertyName(): computedProperty.Default,
				}, &SpecSchemaDefinition{
					Properties: SpecSchemaDefinitionProperties{
						newIntSchemaDefinitionPropertyWithDefaults("origin_port", "", true, false, 80),
						newStringSchemaDefinitionPropertyWithDefaults("protocol", "", true, false, "http"),
						computedProperty,
					},
				}),
			},
			expectedPayload: map[string]interface{}{
				stringProperty.GetTerraformCompliantPropertyName(): stringProperty.Default,
				"object_property": map[string]interface{}{
					"origin_port": int64(80), // this is how ints are stored internally in terraform state
					"protocol":    "http",
				},
			},
		},
		{
			// - Representation of resourceData configuration containing an object which has a property named id
			// {
			//	 object_property = {
			//		id = "someID"
			//	 }
			// }
			name: "properties within objects that are named id and are not readOnly should be included in the payload",
			inputProps: []*SpecSchemaDefinitionProperty{
				newObjectSchemaDefinitionPropertyWithDefaults("object_property", "", true, false, false, map[string]interface{}{
					"id": "someID",
				}, &SpecSchemaDefinition{
					Properties: SpecSchemaDefinitionProperties{
						newStringSchemaDefinitionProperty("id", "", false, false, false, false, false, true, false, false, "someID"),
					},
				}),
			},
			expectedPayload: map[string]interface{}{
				"object_property": map[string]interface{}{
					"id": "someID",
				},
			},
		},
		{
			// - Representation of resourceData configuration containing a complex object (object with other objects)
			// {
			//   string_property = "updatedValue"
			//   property_with_nested_object = [ <-- complex objects are represented in the terraform schema as TypeList with maxElem = 1
			//     {
			//       id = "id"
			//       object_property = {
			//		   some_prop = "someValue"
			//		 }
			//     }
			//   ]
			// }
			name: "nested objects should be added to the payload",
			inputProps: []*SpecSchemaDefinitionProperty{
				stringProperty,
				newObjectSchemaDefinitionPropertyWithDefaults("property_with_nested_object", "", true, false, false, []interface{}{
					map[string]interface{}{
						computedProperty.GetTerraformCompliantPropertyName(): computedProperty.Default,
						"object_property": map[string]interface{}{
							"some_prop": "someValue",
						},
					},
				}, &SpecSchemaDefinition{
					Properties: SpecSchemaDefinitionProperties{
						computedProperty,
						newObjectSchemaDefinitionPropertyWithDefaults("object_property", "", true, false, false, map[string]interface{}{
							"some_prop": "someValue",
						}, &SpecSchemaDefinition{
							Properties: SpecSchemaDefinitionProperties{
								newStringSchemaDefinitionProperty("some_prop", "", true, false, false, false, false, true, false, false, "someValue"),
							},
						}),
					},
				}),
			},
			expectedPayload: map[string]interface{}{
				stringProperty.GetTerraformCompliantPropertyName(): stringProperty.Default,
				"property_with_nested_object": map[string]interface{}{
					"object_property": map[string]interface{}{
						"some_prop": "someValue",
					},
				},
			},
		},
		{
			// - Representation of resourceData configuration containing an array of objects
			// slice_object_property = [
			//   {
			//	   origin_port = 80
			//     protocol = "http"
			//   }
			// ]
			name: "array properties containing objects and are not readOnly should be included in the payload",
			inputProps: []*SpecSchemaDefinitionProperty{
				newListSchemaDefinitionPropertyWithDefaults("slice_object_property", "", true, false, false, []interface{}{
					map[string]interface{}{
						"origin_port": 80,
						"protocol":    "http",
					},
				}, TypeObject, &SpecSchemaDefinition{
					Properties: SpecSchemaDefinitionProperties{
						newIntSchemaDefinitionPropertyWithDefaults("origin_port", "", true, false, 80),
						newStringSchemaDefinitionPropertyWithDefaults("protocol", "", true, false, "http"),
					},
				}),
			},
			expectedPayload: map[string]interface{}{
				"slice_object_property": []interface{}{
					map[string]interface{}{
						"origin_port": 80,
						"protocol":    "http",
					},
				},
			},
		},
		{
			name: "properties with zero values should be included in the payload",
			inputProps: []*SpecSchemaDefinitionProperty{
				intZeroValueProperty,
				numberZeroValueProperty,
				boolZeroValueProperty,
				sliceZeroValueProperty,
			},
			expectedPayload: map[string]interface{}{
				"bool_property":   false,
				"int_property":    0,
				"number_property": float64(0),
				"slice_property":  []interface{}{interface{}(nil)},
			},
		},
	}

	for _, tc := range testCases {
		r, resourceData := testCreateResourceFactory(t, tc.inputProps...)
		payload := r.createPayloadFromLocalStateData(resourceData)
		assert.Equal(t, tc.expectedPayload, payload, tc.name)
	}
}

func TestGetPropertyPayload(t *testing.T) {
	Convey("Given a resource factory"+
		"When populatePayload is called with a nil property"+
		"Then it panics", t, func() {
		input := map[string]interface{}{}
		dataValue := struct{}{}
		resourceFactory := resourceFactory{}
		So(func() { resourceFactory.populatePayload(input, nil, dataValue) }, ShouldPanic)
	})

	Convey("Given a resource factory"+
		"When populatePayload is called with a nil datavalue"+
		"Then it returns an error", t, func() {
		input := map[string]interface{}{}
		resourceFactory := resourceFactory{}
		So(resourceFactory.populatePayload(input, &SpecSchemaDefinitionProperty{Name: "buu"}, nil).Error(), ShouldEqual, `property 'buu' has a nil state dataValue`)
	})

	Convey("Given a resource factory"+
		"When it is called with  non-nil property and value for dataValue which cannot be cast to []interface{}"+
		"Then it panics", t, func() {
		input := map[string]interface{}{}
		dataValue := []bool{}
		property := &SpecSchemaDefinitionProperty{}
		resourceFactory := resourceFactory{}
		So(func() { resourceFactory.populatePayload(input, property, dataValue) }, ShouldPanic)
	})

	Convey("Given the function handleSliceOrArray"+
		"When it is called with an empty slice for dataValue"+
		"Then it should not return an error", t, func() {
		input := map[string]interface{}{}
		dataValue := []interface{}{}
		property := &SpecSchemaDefinitionProperty{}
		resourceFactory := resourceFactory{}
		e := resourceFactory.populatePayload(input, property, dataValue)
		So(e, ShouldBeNil)
	})

	Convey("Given a resource factory initialized with a spec resource with a schema definition containing a string property", t, func() {
		// Use case - string property (terraform configuration pseudo representation below):
		// string_property = "some value"
		r, resourceData := testCreateResourceFactory(t, stringProperty)
		Convey("When populatePayload is called with an empty map, the string property in the resource schema and it's corresponding terraform resourceData state data value", func() {
			payload := map[string]interface{}{}
			dataValue, _ := resourceData.GetOkExists(stringProperty.GetTerraformCompliantPropertyName())
			err := r.populatePayload(payload, stringProperty, dataValue)
			Convey("Then the error should be nil", func() {
				So(err, ShouldBeNil)
			})
			Convey("Then the map returned should not be empty", func() {
				So(payload, ShouldNotBeEmpty)
			})
			Convey("And then payload returned should have the string property", func() {
				So(payload, ShouldContainKey, stringProperty.Name)
			})
			Convey("And then payload returned should have the data value from the state file", func() {
				So(payload[stringProperty.Name], ShouldEqual, stringProperty.Default)
			})
		})
	})

	Convey("Given a resource factory initialized with a schema definition containing an int property", t, func() {
		// Use case - int property (terraform configuration pseudo representation below):
		// int_property = 1234
		r, resourceData := testCreateResourceFactory(t, intProperty)
		Convey("When populatePayload is called with an empty map, the int property in the resource schema  and it's corresponding terraform resourceData state data value", func() {
			payload := map[string]interface{}{}
			dataValue, _ := resourceData.GetOkExists(intProperty.GetTerraformCompliantPropertyName())
			err := r.populatePayload(payload, intProperty, dataValue)
			Convey("Then the error should be nil", func() {
				So(err, ShouldBeNil)
			})
			Convey("Then the map returned should not be empty", func() {
				So(payload, ShouldNotBeEmpty)
			})
			Convey("And then payload returned should have the integer property", func() {
				So(payload, ShouldContainKey, intProperty.Name)
			})
			Convey("And then payload returned should have the data value from the state file", func() {
				So(payload[intProperty.Name], ShouldEqual, intProperty.Default)
			})
		})
	})

	Convey("Given a resource factory initialized with a schema definition containing a number property", t, func() {
		// Use case - number property (terraform configuration pseudo representation below):
		// number_property = 1.1234
		r, resourceData := testCreateResourceFactory(t, numberProperty)
		Convey("When populatePayload is called with an empty map, the number property in the resource schema and it's corresponding terraform resourceData state data value", func() {
			payload := map[string]interface{}{}
			dataValue, _ := resourceData.GetOkExists(numberProperty.GetTerraformCompliantPropertyName())
			err := r.populatePayload(payload, numberProperty, dataValue)
			Convey("Then the error should be nil", func() {
				So(err, ShouldBeNil)
			})
			Convey("Then the map returned should not be empty", func() {
				So(payload, ShouldNotBeEmpty)
			})
			Convey("And then payload returned should have the number property", func() {
				So(payload, ShouldContainKey, numberProperty.Name)
			})
			Convey("And then payload returned should have the data value from the state file", func() {
				So(payload[numberProperty.Name], ShouldEqual, numberProperty.Default)
			})
		})
	})

	Convey("Given a resource factory initialized with a schema definition containing a bool property", t, func() {
		// Use case - bool property (terraform configuration pseudo representation below):
		// bool_property = true
		r, resourceData := testCreateResourceFactory(t, boolProperty)
		Convey("When populatePayload is called with an empty map, the bool property in the resource schema and it's corresponding terraform resourceData state data value", func() {
			payload := map[string]interface{}{}
			dataValue, _ := resourceData.GetOkExists(boolProperty.GetTerraformCompliantPropertyName())
			err := r.populatePayload(payload, boolProperty, dataValue)
			Convey("Then the error should be nil", func() {
				So(err, ShouldBeNil)
			})
			Convey("Then the map returned should not be empty", func() {
				So(payload, ShouldNotBeEmpty)
			})
			Convey("And then payload returned should have the bool property", func() {
				So(payload, ShouldContainKey, boolProperty.Name)
			})
			Convey("And then payload returned should have the data value from the state file", func() {
				So(payload[boolProperty.Name], ShouldEqual, boolProperty.Default)
			})
		})
	})

	Convey("Given a resource factory initialized with a schema definition containing an object property", t, func() {
		// Use case - object property (terraform configuration pseudo representation below):
		// object_property {
		//	 origin_port = 80
		//	 protocol = "http"
		// }
		objectSchemaDefinition := &SpecSchemaDefinition{
			Properties: SpecSchemaDefinitionProperties{
				newIntSchemaDefinitionPropertyWithDefaults("origin_port", "", true, false, 80),
				newStringSchemaDefinitionPropertyWithDefaults("protocol", "", true, false, "http"),
			},
		}
		objectDefault := map[string]interface{}{
			"origin_port": 80,
			"protocol":    "http",
		}
		objectProperty := newObjectSchemaDefinitionPropertyWithDefaults("object_property", "", true, false, false, objectDefault, objectSchemaDefinition)
		r, resourceData := testCreateResourceFactory(t, objectProperty)
		Convey("When populatePayload is called with an empty map, the object property in the resource schema and it's state data value", func() {
			payload := map[string]interface{}{}
			dataValue, _ := resourceData.GetOkExists(objectProperty.GetTerraformCompliantPropertyName())
			err := r.populatePayload(payload, objectProperty, dataValue)
			Convey("Then the error should be nil", func() {
				So(err, ShouldBeNil)
			})
			Convey("Then the map returned should not be empty", func() {
				So(payload, ShouldNotBeEmpty)
			})
			Convey("And then payload returned should have the object property", func() {
				So(payload, ShouldContainKey, objectProperty.Name)
			})
			Convey("And then payload returned should have the data value from the state file", func() {
				So(payload[objectProperty.Name].(map[string]interface{})[objectProperty.SpecSchemaDefinition.Properties[0].Name], ShouldEqual, objectProperty.SpecSchemaDefinition.Properties[0].Default.(int))
				So(payload[objectProperty.Name].(map[string]interface{})[objectProperty.SpecSchemaDefinition.Properties[1].Name], ShouldEqual, objectProperty.SpecSchemaDefinition.Properties[1].Default)
			})
		})
	})

	Convey("Given a resource factory initialized with a schema definition containing an array ob objects property", t, func() {
		// Use case - object property (terraform configuration pseudo representation below):
		// slice_object_property [
		//{
		//	 origin_port = 80
		//	 protocol = "http"
		// }
		// ]
		objectSchemaDefinition := &SpecSchemaDefinition{
			Properties: SpecSchemaDefinitionProperties{
				newIntSchemaDefinitionPropertyWithDefaults("origin_port", "", true, false, 80),
				newStringSchemaDefinitionPropertyWithDefaults("protocol", "", true, false, "http"),
			},
		}
		objectDefault := map[string]interface{}{
			"origin_port": 80,
			"protocol":    "http",
		}
		arrayObjectDefault := []interface{}{
			objectDefault,
		}
		sliceObjectProperty := newListSchemaDefinitionPropertyWithDefaults("slice_object_property", "", true, false, false, arrayObjectDefault, TypeObject, objectSchemaDefinition)
		r, resourceData := testCreateResourceFactory(t, sliceObjectProperty)
		Convey("When populatePayload is called with an empty map, the array of objects property in the resource schema and it's state data value", func() {
			payload := map[string]interface{}{}
			dataValue, _ := resourceData.GetOkExists(sliceObjectProperty.GetTerraformCompliantPropertyName())
			err := r.populatePayload(payload, sliceObjectProperty, dataValue)
			Convey("Then the error should be nil", func() {
				So(err, ShouldBeNil)
			})
			Convey("Then the map returned should not be empty", func() {
				So(payload, ShouldNotBeEmpty)
			})
			Convey("And then payload returned should have the object property", func() {
				So(payload, ShouldContainKey, sliceObjectProperty.Name)
			})
			Convey("And then payload returned should have the data value from the state file properly formatter with the right types", func() {
				// For some reason the data values in the terraform state file are all strings
				So(payload[sliceObjectProperty.Name].([]interface{})[0].(map[string]interface{})[sliceObjectProperty.SpecSchemaDefinition.Properties[0].Name], ShouldEqual, sliceObjectProperty.SpecSchemaDefinition.Properties[0].Default.(int))
				So(payload[sliceObjectProperty.Name].([]interface{})[0].(map[string]interface{})[sliceObjectProperty.SpecSchemaDefinition.Properties[1].Name], ShouldEqual, sliceObjectProperty.SpecSchemaDefinition.Properties[1].Default)
			})
		})
	})

	Convey("Given a resource factory initialized with a schema definition containing a slice of strings property", t, func() {
		// Use case - slice of srings (terraform configuration pseudo representation below):
		// slice_property = ["some_value"]
		r, resourceData := testCreateResourceFactory(t, slicePrimitiveProperty)
		Convey("When populatePayload is called with an empty map, the slice of strings property in the resource schema and it's state data value", func() {
			payload := map[string]interface{}{}
			dataValue, _ := resourceData.GetOkExists(slicePrimitiveProperty.GetTerraformCompliantPropertyName())
			err := r.populatePayload(payload, slicePrimitiveProperty, dataValue)
			Convey("Then the error should be nil", func() {
				So(err, ShouldBeNil)
			})
			Convey("Then the map returned should not be empty", func() {
				So(payload, ShouldNotBeEmpty)
			})
			Convey("And then payload returned should have the object property", func() {
				So(payload, ShouldContainKey, slicePrimitiveProperty.Name)
			})
			Convey("And then payload returned should have the data value from the state file", func() {
				So(payload[slicePrimitiveProperty.Name].([]interface{})[0], ShouldEqual, slicePrimitiveProperty.Default.([]interface{})[0])
			})
		})
	})

	Convey("Given a resource factory initialized with a spec resource with a property schema definition containing one nested struct", t, func() {
		// Use case - object with nested objects (terraform configuration pseudo representation below):
		// property_with_nested_object {
		//	id = "id",
		//	nested_object {
		//		origin_port = 80
		//		protocol = "http"
		//	}
		//}
		nestedObjectSchemaDefinition := &SpecSchemaDefinition{
			Properties: SpecSchemaDefinitionProperties{
				newIntSchemaDefinitionPropertyWithDefaults("origin_port", "", true, false, 80),
				newStringSchemaDefinitionPropertyWithDefaults("protocol", "", true, false, "http"),
			},
		}
		nestedObjectDefault := map[string]interface{}{
			"origin_port": nestedObjectSchemaDefinition.Properties[0].Default,
			"protocol":    nestedObjectSchemaDefinition.Properties[1].Default,
		}
		nestedObject := newObjectSchemaDefinitionPropertyWithDefaults("nested_object", "", true, false, false, nestedObjectDefault, nestedObjectSchemaDefinition)
		propertyWithNestedObjectSchemaDefinition := &SpecSchemaDefinition{
			Properties: SpecSchemaDefinitionProperties{
				idProperty,
				nestedObject,
			},
		}
		// Tag(NestedStructsWorkaround)
		// Note: This is the workaround needed to support properties with nested structs. The current Terraform sdk version
		// does not support this now, hence the suggestion from the Terraform maintainer was to use a list of map[string]interface{}
		// with the list containing just one element. The below represents the internal representation of the terraform state
		// for an object property that contains other objects
		propertyWithNestedObjectDefault := []interface{}{
			map[string]interface{}{
				"id":            propertyWithNestedObjectSchemaDefinition.Properties[0].Default,
				"nested_object": propertyWithNestedObjectSchemaDefinition.Properties[1].Default,
			},
		}
		expectedPropertyWithNestedObjectName := "property_with_nested_object"
		propertyWithNestedObject := newObjectSchemaDefinitionPropertyWithDefaults(expectedPropertyWithNestedObjectName, "", true, false, false, propertyWithNestedObjectDefault, propertyWithNestedObjectSchemaDefinition)
		r, resourceData := testCreateResourceFactory(t, propertyWithNestedObject)
		Convey("When populatePayload is called a slice with >1 dataValue, it complains", func() {
			err := r.populatePayload(map[string]interface{}{}, propertyWithNestedObject, []interface{}{"foo", "bar", "baz"})
			So(err.Error(), ShouldEqual, "something is really wrong here...an object property with nested objects should have exactly one elem in the terraform state list")

		})
		Convey("When populatePayload is called a slice with <1 dataValue, it complains", func() {
			err := r.populatePayload(map[string]interface{}{}, propertyWithNestedObject, []interface{}{})
			So(err.Error(), ShouldEqual, "something is really wrong here...an object property with nested objects should have exactly one elem in the terraform state list")

		})

		Convey("When populatePayload is called with an empty map, the property with nested object in the resource schema and it's corresponding terraform resourceData state data value", func() {
			payload := map[string]interface{}{}
			dataValue, _ := resourceData.GetOkExists(propertyWithNestedObject.GetTerraformCompliantPropertyName())
			err := r.populatePayload(payload, propertyWithNestedObject, dataValue)
			Convey("Then the error should be nil", func() {
				So(err, ShouldBeNil)
			})
			Convey("Then the map returned should not be empty", func() {
				So(payload, ShouldNotBeEmpty)
			})
			Convey("Then the map returned should contain the 'property_with_nested_object' property", func() {
				So(payload, ShouldContainKey, expectedPropertyWithNestedObjectName)
			})
			topLevel := payload[expectedPropertyWithNestedObjectName].(map[string]interface{})
			Convey("And then payload returned should have the id property with the right value", func() {
				So(topLevel, ShouldContainKey, idProperty.Name)
				So(topLevel[idProperty.Name], ShouldEqual, propertyWithNestedObjectSchemaDefinition.Properties[0].Default)
			})
			Convey("And then payload returned should have the nested_object object property with the right value", func() {
				So(topLevel, ShouldContainKey, nestedObject.Name)
				nestedLevel := topLevel[nestedObject.Name].(map[string]interface{})
				So(nestedLevel["origin_port"], ShouldEqual, propertyWithNestedObjectSchemaDefinition.Properties[1].Default.(map[string]interface{})["origin_port"])
				So(nestedLevel["protocol"], ShouldEqual, propertyWithNestedObjectSchemaDefinition.Properties[1].Default.(map[string]interface{})["protocol"])
			})
		})
	})

	Convey("Given a resource factory initialized with a spec resource with a property schema definition containing one nested struct but having terraform property names that are not valid within the resource definition", t, func() {
		// Use case -  crappy path while getting properties paylod for properties which do not exists in nested objects
		nestedObjectSchemaDefinition := &SpecSchemaDefinition{
			Properties: SpecSchemaDefinitionProperties{
				newStringSchemaDefinitionPropertyWithDefaults("protocol", "", true, false, "http"),
			},
		}
		nestedObjectNoTFCompliantName := map[string]interface{}{
			"badprotocoldoesntexist": nestedObjectSchemaDefinition.Properties[0].Default,
		}
		nestedObjectNotTFCompliant := newObjectSchemaDefinitionPropertyWithDefaults("nested_object_not_compliant", "", true, false, false, nestedObjectNoTFCompliantName, nestedObjectSchemaDefinition)
		propertyWithNestedObjectSchemaDefinition := &SpecSchemaDefinition{
			Properties: SpecSchemaDefinitionProperties{
				idProperty,
				nestedObjectNotTFCompliant,
			},
		}
		propertyWithNestedObjectDefault := []interface{}{
			map[string]interface{}{
				"id":                          propertyWithNestedObjectSchemaDefinition.Properties[0].Default,
				"nested_object_not_compliant": propertyWithNestedObjectSchemaDefinition.Properties[1].Default,
			},
		}
		expectedPropertyWithNestedObjectName := "property_with_nested_object"
		propertyWithNestedObject := newObjectSchemaDefinitionPropertyWithDefaults(expectedPropertyWithNestedObjectName, "", true, false, false, propertyWithNestedObjectDefault, propertyWithNestedObjectSchemaDefinition)
		r, resourceData := testCreateResourceFactory(t, propertyWithNestedObject)
		Convey("When populatePayload is called with an empty map, the property with nested object in the resource schema and it's corresponding terraform resourceData state data value", func() {
			payload := map[string]interface{}{}
			dataValue, _ := resourceData.GetOkExists(propertyWithNestedObject.GetTerraformCompliantPropertyName())
			err := r.populatePayload(payload, propertyWithNestedObject, dataValue)
			Convey("Then the error should not be nil", func() {
				So(err.Error(), ShouldEqual, "property with terraform name 'badprotocoldoesntexist' not existing in resource schema definition")
			})
			Convey("Then the map returned should be empty", func() {
				So(payload, ShouldBeEmpty)
			})
		})
	})

	Convey("Given a resource factory initialized with a spec resource with a property schema definition containing a slice of objects with an invalid slice name definition", t, func() {
		// Use case -  crappy path while getting properties paylod for properties which do not exists in slices
		objectSchemaDefinition := &SpecSchemaDefinition{}
		arrayObjectDefault := []interface{}{
			map[string]interface{}{
				"protocol": "http",
			},
		}
		sliceObjectProperty := newListSchemaDefinitionPropertyWithDefaults("slice_object_property_doesn_not_exists", "", true, false, false, arrayObjectDefault, TypeObject, objectSchemaDefinition)

		r, resourceData := testCreateResourceFactory(t, sliceObjectProperty)

		Convey("When populatePayload is called with an empty map, the property slice of objects in the resource schema are not found", func() {
			payload := map[string]interface{}{}
			dataValue, _ := resourceData.GetOkExists(sliceObjectProperty.GetTerraformCompliantPropertyName())
			err := r.populatePayload(payload, sliceObjectProperty, dataValue)
			Convey("Then the error should not be nil", func() {
				So(err.Error(), ShouldEqual, "property 'slice_object_property_doesn_not_exists' has a nil state dataValue")
			})
			Convey("Then the map returned should be empty", func() {
				So(payload, ShouldBeEmpty)
			})
		})
	})
}

func TestGetStatusValueFromPayload(t *testing.T) {
	Convey("Given a swagger schema definition that has an status property that is not an object", t, func() {
		specResource := newSpecStubResource(
			"resourceName",
			"/v1/resource",
			false,
			&SpecSchemaDefinition{
				Properties: SpecSchemaDefinitionProperties{
					&SpecSchemaDefinitionProperty{
						Name:     statusDefaultPropertyName,
						Type:     TypeString,
						ReadOnly: true,
					},
				},
			})
		r := resourceFactory{
			openAPIResource: specResource,
		}
		Convey("When getStatusValueFromPayload method is called with a payload that also has a 'status' field in the root level", func() {
			expectedStatusValue := "someValue"
			payload := map[string]interface{}{
				statusDefaultPropertyName: expectedStatusValue,
			}
			statusField, err := r.getStatusValueFromPayload(payload)
			Convey("Then the error returned should be nil", func() {
				So(err, ShouldBeNil)
			})
			Convey("Then the value returned should contain the name of the property 'status'", func() {
				So(statusField, ShouldEqual, expectedStatusValue)
			})
		})

		Convey("When getStatusValueFromPayload method is called with a payload that does not have status field", func() {
			payload := map[string]interface{}{
				"someOtherPropertyName": "arggg",
			}
			_, err := r.getStatusValueFromPayload(payload)
			Convey("Then the error returned should NOT be nil", func() {
				So(err, ShouldNotBeNil)
			})
			Convey("Then the error message should be", func() {
				So(err.Error(), ShouldEqual, "payload does not match resouce schema, could not find the status field: [status]")
			})
		})

		Convey("When getStatusValueFromPayload method is called with a payload that has a status field but the value is not supported", func() {
			payload := map[string]interface{}{
				statusDefaultPropertyName: 12, // this value is not supported, only strings and maps (for nested properties within an object) are supported
			}
			_, err := r.getStatusValueFromPayload(payload)
			Convey("Then the error returned should NOT be nil", func() {
				So(err, ShouldNotBeNil)
			})
			Convey("Then the error message should be", func() {
				So(err.Error(), ShouldEqual, "status property value '[status]' does not have a supported type [string/map]")
			})
		})
	})

	Convey("Given a swagger schema definition that has an status property that IS an object", t, func() {
		expectedStatusProperty := "some-other-property-holding-status"
		specResource := newSpecStubResource(
			"resourceName",
			"/v1/resource",
			false,
			&SpecSchemaDefinition{
				Properties: SpecSchemaDefinitionProperties{
					&SpecSchemaDefinitionProperty{
						Name:     "id",
						Type:     TypeString,
						ReadOnly: true,
					},
					&SpecSchemaDefinitionProperty{
						Name:     statusDefaultPropertyName,
						Type:     TypeObject,
						ReadOnly: true,
						SpecSchemaDefinition: &SpecSchemaDefinition{
							Properties: SpecSchemaDefinitionProperties{
								&SpecSchemaDefinitionProperty{
									Name:               expectedStatusProperty,
									Type:               TypeString,
									ReadOnly:           true,
									IsStatusIdentifier: true,
								},
							},
						},
					},
				},
			})
		r := resourceFactory{
			openAPIResource: specResource,
		}
		Convey("When getStatusValueFromPayload method is called with a payload that has an status object property inside which there's an status property", func() {
			expectedStatusValue := "someStatusValue"
			payload := map[string]interface{}{
				statusDefaultPropertyName: map[string]interface{}{
					expectedStatusProperty: expectedStatusValue,
				},
			}
			statusField, err := r.getStatusValueFromPayload(payload)
			Convey("Then the error returned should be nil", func() {
				So(err, ShouldBeNil)
			})
			Convey("Then the value returned should contain the name of the property 'status'", func() {
				So(statusField, ShouldEqual, expectedStatusValue)
			})
		})
	})

}

func TestGetResourceDataOKExists(t *testing.T) {
	Convey("Given a resource factory initialized with a spec resource with some schema definition and resource data", t, func() {
		r, resourceData := testCreateResourceFactory(t, stringProperty, stringWithPreferredNameProperty)
		Convey("When getResourceDataOKExists is called with a schema definition property name that exists in terraform resource data object", func() {
			value, exists := r.getResourceDataOKExists(stringProperty.Name, resourceData)
			Convey("Then the bool returned should be true", func() {
				So(exists, ShouldBeTrue)
			})
			Convey("And then expectedValue should equal", func() {
				So(value, ShouldEqual, stringProperty.Default)
			})
		})

		Convey("When getResourceDataOKExists is called with a schema definition property name that has a preferred name and that exists in terraform resource data object", func() {
			value, exists := r.getResourceDataOKExists(stringWithPreferredNameProperty.Name, resourceData)
			Convey("Then the bool returned should be true", func() {
				So(exists, ShouldBeTrue)
			})
			Convey("And then expectedValue should equal", func() {
				So(value, ShouldEqual, stringWithPreferredNameProperty.Default)
			})
		})

		Convey("When getResourceDataOKExists is called with a schema definition property name that DOES NOT exists in terraform resource data object", func() {
			_, exists := r.getResourceDataOKExists("nonExistingProperty", resourceData)
			Convey("Then the bool returned should be true", func() {
				So(exists, ShouldBeFalse)
			})
		})
	})

	Convey("Given a resource factory initialized with a spec resource with some schema definition and resource data", t, func() {
		var stringPropertyWithNonCompliantName = newStringSchemaDefinitionPropertyWithDefaults("stringProperty", "", true, false, "updatedValue")
		r, resourceData := testCreateResourceFactory(t, stringPropertyWithNonCompliantName)
		Convey("When getResourceDataOKExists is called with a schema definition property name that exists in terraform resource data object", func() {
			value, exists := r.getResourceDataOKExists(stringPropertyWithNonCompliantName.Name, resourceData)
			Convey("Then the bool returned should be true", func() {
				So(exists, ShouldBeTrue)
			})
			Convey("And then expectedValue should equal", func() {
				So(value, ShouldEqual, stringPropertyWithNonCompliantName.Default)
			})
		})
	})
}

// testCreateResourceFactoryWithID configures the resourceData with the Id field. This is used for tests that rely on the
// resource state to be fully created. For instance, update or delete operations.
func testCreateResourceFactoryWithID(t *testing.T, idSchemaDefinitionProperty *SpecSchemaDefinitionProperty, schemaDefinitionProperties ...*SpecSchemaDefinitionProperty) (resourceFactory, *schema.ResourceData) {
	schemaDefinitionProperties = append(schemaDefinitionProperties, idSchemaDefinitionProperty)
	resourceFactory, resourceData := testCreateResourceFactory(t, schemaDefinitionProperties...)
	resourceData.SetId(idSchemaDefinitionProperty.Default.(string))
	return resourceFactory, resourceData
}

// testCreateResourceFactory configures the resourceData with some properties.
func testCreateResourceFactory(t *testing.T, schemaDefinitionProperties ...*SpecSchemaDefinitionProperty) (resourceFactory, *schema.ResourceData) {
	testSchema := newTestSchema(schemaDefinitionProperties...)
	resourceData := testSchema.getResourceData(t)
	specResource := newSpecStubResourceWithOperations("resourceName", "/v1/resource", false, testSchema.getSchemaDefinition(), &specResourceOperation{}, &specResourceOperation{}, &specResourceOperation{}, &specResourceOperation{})
	return newResourceFactory(specResource), resourceData
}

func testCreateSubResourceFactory(t *testing.T, path string, parentResourceNames []string, fullParentResourceName string, idSchemaDefinitionProperty *SpecSchemaDefinitionProperty, schemaDefinitionProperties ...*SpecSchemaDefinitionProperty) (resourceFactory, *schema.ResourceData) {
	testSchema := newTestSchema(schemaDefinitionProperties...)
	resourceData := testSchema.getResourceData(t)
	resourceData.SetId(idSchemaDefinitionProperty.Default.(string))
	specResource := newSpecStubResourceWithOperations("subResourceName", path, false, testSchema.getSchemaDefinition(), &specResourceOperation{}, &specResourceOperation{}, &specResourceOperation{}, &specResourceOperation{})
	specResource.parentResourceNames = parentResourceNames
	specResource.fullParentResourceName = fullParentResourceName
	return newResourceFactory(specResource), resourceData
}
