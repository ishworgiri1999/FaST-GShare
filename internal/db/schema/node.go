package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/KontonGu/FaST-GShare/internal/db/schema/mixin"
	"github.com/google/uuid"
)

type Node struct {
	ent.Schema
}

func (Node) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Unique().
			Immutable(),

		field.String("hostname").
			Comment("Resource UUID"),

		field.String("ip_address").
			Comment("Node IP Address"),
	}
}

func (Node) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("resources", Resource.Type),
	}
}

func (Node) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("node_name"),
	}
}
func (Node) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.CreateTime{},
	}
}
