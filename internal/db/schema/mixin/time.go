package mixin

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
)

// CreateTime adds created_at field.
type CreateTime struct{ mixin.Schema }

// Fields of the create time mixin.
func (CreateTime) Fields() []ent.Field {
	return []ent.Field{
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

// create time mixin must implement `Mixin` interface.
var _ ent.Mixin = (*CreateTime)(nil)

// UpdatedAt adds updated_at field.
type UpdateTime struct{ mixin.Schema }

// Fields of the update time mixin.
func (UpdateTime) Fields() []ent.Field {
	return []ent.Field{
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// update time mixin must implement `Mixin` interface.
var _ ent.Mixin = (*UpdateTime)(nil)

// Time composes create/update time mixin.
type Time struct{ mixin.Schema }

// Fields of the time mixin.
func (Time) Fields() []ent.Field {
	return append(
		CreateTime{}.Fields(),
		UpdateTime{}.Fields()...,
	)
}

// time mixin must implement `Mixin` interface.
var _ ent.Mixin = (*Time)(nil)

// DeleteTime adds deleted_at field.\
type DeleteTime struct{ mixin.Schema }

// Fields of the delete time mixin.
func (DeleteTime) Fields() []ent.Field {
	return []ent.Field{
		field.Time("deleted_at").
			Optional().
			Nillable(),
	}
}

// delete time mixin must implement `Mixin` interface.
var _ ent.Mixin = (*DeleteTime)(nil)
