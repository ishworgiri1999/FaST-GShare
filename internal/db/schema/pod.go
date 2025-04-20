package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/KontonGu/FaST-GShare/internal/db/schema/mixin"
	"github.com/google/uuid"
)

type Pod struct {
	ent.Schema
}

func (Pod) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Unique().
			Immutable(),
		field.String("name").
			Comment("Pod Name"),

		field.String("resource_id").
			Comment("Resource ID"),
	}
}

func (Pod) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("resource", Resource.Type),
	}
}

func (Pod) Indexes() []ent.Index {
	return []ent.Index{}
}
func (Pod) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.CreateTime{},
	}
}
