package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/KontonGu/FaST-GShare/internal/db/schema/mixin"
	"github.com/google/uuid"
)

type GPU struct {
	ent.Schema
}

func (GPU) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("gpu_type", uuid.UUID{}).
			Default(uuid.New).
			Unique().
			Immutable(),

		field.Int32("tflops").
			Comment("TFLOPS"),
	}
}

func (GPU) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("pods", Pod.Type).
			Ref("resource"),
	}
}

func (GPU) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("node_name").Unique(),
	}
}
func (GPU) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.CreateTime{},
	}
}
