package ldclient

import (
	"time"
)

type User struct {
	Key       *string                      `json:"key,omitempty" bson:"key,omitempty"`
	Secondary *string                      `json:"secondary,omitempty" bson:"secondary,omitempty"`
	Ip        *string                      `json:"ip,omitempty" bson:"ip,omitempty"`
	Country   *string                      `json:"country,omitempty" bson:"country,omitempty"`
	Email     *string                      `json:"email,omitempty" bson:"email,omitempty"`
	FirstName *string                      `json:"firstName,omitempty" bson:"firstName,omitempty"`
	LastName  *string                      `json:"lastName,omitempty" bson:"lastName,omitempty"`
	Avatar    *string                      `json:"avatar,omitempty" bson:"avatar,omitempty"`
	Name      *string                      `json:"name,omitempty" bson:"name,omitempty"`
	Anonymous *bool                        `json:"anonymous,omitempty" bson:"anonymous,omitempty"`
	Custom    *map[string]interface{}      `json:"custom,omitempty" bson:"custom,omitempty"`
	Derived   map[string]*DerivedAttribute `json:"derived,omitempty" bson:"derived,omitempty"`
}

type DerivedAttribute struct {
	Value       interface{} `json:"value" bson:"value"`
	LastDerived time.Time   `json:"lastDerived" bson:"lastDerived"`
}
