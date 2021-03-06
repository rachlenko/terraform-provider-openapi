package openapi

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewAPIKeyHeaderSecurityDefinition(t *testing.T) {
	Convey("Given a name and an apikey name", t, func() {
		name := "name"
		apiKeyName := "apiKey_name"
		Convey("When newAPIKeyHeaderSecurityDefinition method is called", func() {
			apiKeyHeaderSecurityDefinition := newAPIKeyHeaderSecurityDefinition(name, apiKeyName)
			Convey("Then the apiKeyHeaderAuthenticator should comply with specAPIKeyAuthenticator interface", func() {
				var _ SpecSecurityDefinition = apiKeyHeaderSecurityDefinition
			})
		})
	})
}

func TestAPIKeyHeaderSecurityDefinitionGetName(t *testing.T) {
	Convey("Given an APIKeyHeaderSecurityDefinition", t, func() {
		expectedName := "apikey_name"
		apiKeyHeaderSecurityDefinition := newAPIKeyHeaderSecurityDefinition(expectedName, authorizationHeader)
		Convey("When GetTerraformConfigurationName method is called", func() {
			name := apiKeyHeaderSecurityDefinition.getName()
			Convey("Then the result should match the original name", func() {
				So(name, ShouldEqual, expectedName)
			})
		})
	})
}

func TestAPIKeyHeaderSecurityDefinitionGetType(t *testing.T) {
	Convey("Given an APIKeyHeaderSecurityDefinition", t, func() {
		apiKeyHeaderSecurityDefinition := newAPIKeyHeaderSecurityDefinition("apikey_name", authorizationHeader)
		Convey("When getType method is called", func() {
			secDefType := apiKeyHeaderSecurityDefinition.getType()
			Convey("Then the result should be securityDefinitionAPIKey", func() {
				So(secDefType, ShouldEqual, securityDefinitionAPIKey)
			})
		})
	})
}

func TestAPIKeyHeaderSecurityDefinitionGetTerraformConfigurationName(t *testing.T) {
	Convey("Given an APIKeyHeaderSecurityDefinition with a compliant name", t, func() {
		apiKeyHeaderSecurityDefinition := newAPIKeyHeaderSecurityDefinition("apikey_name", authorizationHeader)
		Convey("When GetTerraformConfigurationName method is called", func() {
			secDefTfName := apiKeyHeaderSecurityDefinition.GetTerraformConfigurationName()
			Convey("Then the result should be securityDefinitionAPIKey", func() {
				So(secDefTfName, ShouldEqual, "apikey_name")
			})
		})
	})

	Convey("Given an APIKeyHeaderSecurityDefinition with a NON compliant name", t, func() {
		apiKeyHeaderSecurityDefinition := newAPIKeyHeaderSecurityDefinition("nonCompliantName", authorizationHeader)
		Convey("When GetTerraformConfigurationName method is called", func() {
			secDefTfName := apiKeyHeaderSecurityDefinition.GetTerraformConfigurationName()
			Convey("Then the result should be securityDefinitionAPIKey", func() {
				So(secDefTfName, ShouldEqual, "non_compliant_name")
			})
		})
	})
}

func TestAPIKeyHeaderSecurityDefinitionGetAPIKey(t *testing.T) {
	Convey("Given an APIKeyHeaderSecurityDefinition", t, func() {
		expectedAPIKey := authorizationHeader
		apiKeyHeaderSecurityDefinition := newAPIKeyHeaderSecurityDefinition("apiKeyName", expectedAPIKey)
		Convey("When GetTerraformConfigurationName method is called", func() {
			apiKey := apiKeyHeaderSecurityDefinition.getAPIKey()
			Convey("Then the result should contain the right apikey name and location", func() {
				So(apiKey.Name, ShouldEqual, expectedAPIKey)
				So(apiKey.In, ShouldEqual, inHeader)
			})
		})
	})
}

func TestAPIKeyHeaderSecurityDefinitionBuildValue(t *testing.T) {
	Convey("Given an APIKeyHeaderSecurityDefinition", t, func() {
		expectedAPIKey := authorizationHeader
		apiKeyHeaderSecurityDefinition := newAPIKeyHeaderSecurityDefinition("apiKeyName", expectedAPIKey)
		Convey("When GetTerraformConfigurationName method is called", func() {
			expectedValue := "someValue"
			value := apiKeyHeaderSecurityDefinition.buildValue("someValue")
			Convey("Then the value should be the expected value with no modifications", func() {
				So(value, ShouldEqual, expectedValue)
			})
		})
	})
}

func TestAPIKeyHeaderSecurityDefinitionValidate(t *testing.T) {
	Convey("Given an APIKeyHeaderSecurityDefinition with a security definition name and an apiKeyName", t, func() {
		apiKeyHeaderSecurityDefinition := newAPIKeyHeaderSecurityDefinition("apiKeyName", authorizationHeader)
		Convey("When validate method is called", func() {
			err := apiKeyHeaderSecurityDefinition.validate()
			Convey("Then the error returned should be nil", func() {
				So(err, ShouldBeNil)
			})
		})
	})
	Convey("Given an APIKeyHeaderSecurityDefinition with an empty security definition name and an apiKeyName", t, func() {
		apiKeyHeaderSecurityDefinition := newAPIKeyHeaderSecurityDefinition("", authorizationHeader)
		Convey("When validate method is called", func() {
			err := apiKeyHeaderSecurityDefinition.validate()
			Convey("Then the error returned should NOT be nil", func() {
				So(err, ShouldNotBeNil)
			})
			Convey("Then the error message should match the expected", func() {
				So(err.Error(), ShouldEqual, "specAPIKeyHeaderSecurityDefinition missing mandatory security definition name")
			})
		})
	})
	Convey("Given an APIKeyHeaderSecurityDefinition with a security definition name and an empty apiKeyName", t, func() {
		apiKeyHeaderSecurityDefinition := newAPIKeyHeaderSecurityDefinition("apiKeyName", "")
		Convey("When validate method is called", func() {
			err := apiKeyHeaderSecurityDefinition.validate()
			Convey("Then the error returned should NOT be nil", func() {
				So(err, ShouldNotBeNil)
			})
			Convey("Then the error message should match the expected", func() {
				So(err.Error(), ShouldEqual, "specAPIKeyHeaderSecurityDefinition missing mandatory apiKey name")
			})
		})
	})
}
