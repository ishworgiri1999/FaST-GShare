package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/KontonGu/FaST-GShare/internal/db/schema/mixin"
	"github.com/KontonGu/FaST-GShare/pkg/types"
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

		field.Int64("total_memory").
			Comment("Total Memory").
			Positive(),

		field.Int64("used_memory").
			Comment("Used Memory").
			Positive(),

		field.UUID("node_id", uuid.UUID{}).
			Comment("Node ID"),

		field.String("gpu_type").
			Comment("GPU Type"),

		field.Int("sm_count").
			Comment("SM Count").
			Positive(),

		field.Bool("virtual").
			Comment("Virtual"),

		field.Bool("provisioned").
			Comment("Provisioned"),

		field.String("allocation_type").
			GoType(types.AllocationType("")),

		field.Int("sm_partition").
			Comment("SM Partition"),
	}
}

func (Resource) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("node", Node.Type).
			Ref("resources").
			Unique().
			Field("node_id").
			Required(),

		edge.From("pods", Pod.Type).
			Ref("resource"),

		edge.From("gpu", GPU.Type).
			Ref("resources").
			Field("gpu_type").
			Required(),
	}
}

func (Resource) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("node_name"),
	}
}
func (Resource) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.CreateTime{},
	}
}
