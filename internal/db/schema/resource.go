package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/KontonGu/FaST-GShare/internal/db/schema/mixin"
	"github.com/google/uuid"
)

type Resource struct {
	ent.Schema
}

func (Resource) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Unique().
			Immutable(),

		field.String("resource_uuid").
			Comment("Resource UUID"),
		field.String("node_name").
			Comment("Node name"),
	}
}

func (Resource) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("pods", Pod.Type).
			Ref("resource"),
	}
}

func (Resource) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("node_name").Unique(),
	}
}
func (Resource) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.CreateTime{},
	}
}
