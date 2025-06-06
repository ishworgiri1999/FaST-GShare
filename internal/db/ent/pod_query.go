// Code generated by ent, DO NOT EDIT.

package ent

import (
	"context"
	"database/sql/driver"
	"fmt"
	"math"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqlgraph"
	"entgo.io/ent/schema/field"
	"github.com/KontonGu/FaST-GShare/internal/db/ent/pod"
	"github.com/KontonGu/FaST-GShare/internal/db/ent/predicate"
	"github.com/KontonGu/FaST-GShare/internal/db/ent/resource"
	"github.com/google/uuid"
)

// PodQuery is the builder for querying Pod entities.
type PodQuery struct {
	config
	ctx          *QueryContext
	order        []pod.OrderOption
	inters       []Interceptor
	predicates   []predicate.Pod
	withResource *ResourceQuery
	modifiers    []func(*sql.Selector)
	// intermediate query (i.e. traversal path).
	sql  *sql.Selector
	path func(context.Context) (*sql.Selector, error)
}

// Where adds a new predicate for the PodQuery builder.
func (pq *PodQuery) Where(ps ...predicate.Pod) *PodQuery {
	pq.predicates = append(pq.predicates, ps...)
	return pq
}

// Limit the number of records to be returned by this query.
func (pq *PodQuery) Limit(limit int) *PodQuery {
	pq.ctx.Limit = &limit
	return pq
}

// Offset to start from.
func (pq *PodQuery) Offset(offset int) *PodQuery {
	pq.ctx.Offset = &offset
	return pq
}

// Unique configures the query builder to filter duplicate records on query.
// By default, unique is set to true, and can be disabled using this method.
func (pq *PodQuery) Unique(unique bool) *PodQuery {
	pq.ctx.Unique = &unique
	return pq
}

// Order specifies how the records should be ordered.
func (pq *PodQuery) Order(o ...pod.OrderOption) *PodQuery {
	pq.order = append(pq.order, o...)
	return pq
}

// QueryResource chains the current query on the "resource" edge.
func (pq *PodQuery) QueryResource() *ResourceQuery {
	query := (&ResourceClient{config: pq.config}).Query()
	query.path = func(ctx context.Context) (fromU *sql.Selector, err error) {
		if err := pq.prepareQuery(ctx); err != nil {
			return nil, err
		}
		selector := pq.sqlQuery(ctx)
		if err := selector.Err(); err != nil {
			return nil, err
		}
		step := sqlgraph.NewStep(
			sqlgraph.From(pod.Table, pod.FieldID, selector),
			sqlgraph.To(resource.Table, resource.FieldID),
			sqlgraph.Edge(sqlgraph.M2M, false, pod.ResourceTable, pod.ResourcePrimaryKey...),
		)
		fromU = sqlgraph.SetNeighbors(pq.driver.Dialect(), step)
		return fromU, nil
	}
	return query
}

// First returns the first Pod entity from the query.
// Returns a *NotFoundError when no Pod was found.
func (pq *PodQuery) First(ctx context.Context) (*Pod, error) {
	nodes, err := pq.Limit(1).All(setContextOp(ctx, pq.ctx, ent.OpQueryFirst))
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, &NotFoundError{pod.Label}
	}
	return nodes[0], nil
}

// FirstX is like First, but panics if an error occurs.
func (pq *PodQuery) FirstX(ctx context.Context) *Pod {
	node, err := pq.First(ctx)
	if err != nil && !IsNotFound(err) {
		panic(err)
	}
	return node
}

// FirstID returns the first Pod ID from the query.
// Returns a *NotFoundError when no Pod ID was found.
func (pq *PodQuery) FirstID(ctx context.Context) (id uuid.UUID, err error) {
	var ids []uuid.UUID
	if ids, err = pq.Limit(1).IDs(setContextOp(ctx, pq.ctx, ent.OpQueryFirstID)); err != nil {
		return
	}
	if len(ids) == 0 {
		err = &NotFoundError{pod.Label}
		return
	}
	return ids[0], nil
}

// FirstIDX is like FirstID, but panics if an error occurs.
func (pq *PodQuery) FirstIDX(ctx context.Context) uuid.UUID {
	id, err := pq.FirstID(ctx)
	if err != nil && !IsNotFound(err) {
		panic(err)
	}
	return id
}

// Only returns a single Pod entity found by the query, ensuring it only returns one.
// Returns a *NotSingularError when more than one Pod entity is found.
// Returns a *NotFoundError when no Pod entities are found.
func (pq *PodQuery) Only(ctx context.Context) (*Pod, error) {
	nodes, err := pq.Limit(2).All(setContextOp(ctx, pq.ctx, ent.OpQueryOnly))
	if err != nil {
		return nil, err
	}
	switch len(nodes) {
	case 1:
		return nodes[0], nil
	case 0:
		return nil, &NotFoundError{pod.Label}
	default:
		return nil, &NotSingularError{pod.Label}
	}
}

// OnlyX is like Only, but panics if an error occurs.
func (pq *PodQuery) OnlyX(ctx context.Context) *Pod {
	node, err := pq.Only(ctx)
	if err != nil {
		panic(err)
	}
	return node
}

// OnlyID is like Only, but returns the only Pod ID in the query.
// Returns a *NotSingularError when more than one Pod ID is found.
// Returns a *NotFoundError when no entities are found.
func (pq *PodQuery) OnlyID(ctx context.Context) (id uuid.UUID, err error) {
	var ids []uuid.UUID
	if ids, err = pq.Limit(2).IDs(setContextOp(ctx, pq.ctx, ent.OpQueryOnlyID)); err != nil {
		return
	}
	switch len(ids) {
	case 1:
		id = ids[0]
	case 0:
		err = &NotFoundError{pod.Label}
	default:
		err = &NotSingularError{pod.Label}
	}
	return
}

// OnlyIDX is like OnlyID, but panics if an error occurs.
func (pq *PodQuery) OnlyIDX(ctx context.Context) uuid.UUID {
	id, err := pq.OnlyID(ctx)
	if err != nil {
		panic(err)
	}
	return id
}

// All executes the query and returns a list of Pods.
func (pq *PodQuery) All(ctx context.Context) ([]*Pod, error) {
	ctx = setContextOp(ctx, pq.ctx, ent.OpQueryAll)
	if err := pq.prepareQuery(ctx); err != nil {
		return nil, err
	}
	qr := querierAll[[]*Pod, *PodQuery]()
	return withInterceptors[[]*Pod](ctx, pq, qr, pq.inters)
}

// AllX is like All, but panics if an error occurs.
func (pq *PodQuery) AllX(ctx context.Context) []*Pod {
	nodes, err := pq.All(ctx)
	if err != nil {
		panic(err)
	}
	return nodes
}

// IDs executes the query and returns a list of Pod IDs.
func (pq *PodQuery) IDs(ctx context.Context) (ids []uuid.UUID, err error) {
	if pq.ctx.Unique == nil && pq.path != nil {
		pq.Unique(true)
	}
	ctx = setContextOp(ctx, pq.ctx, ent.OpQueryIDs)
	if err = pq.Select(pod.FieldID).Scan(ctx, &ids); err != nil {
		return nil, err
	}
	return ids, nil
}

// IDsX is like IDs, but panics if an error occurs.
func (pq *PodQuery) IDsX(ctx context.Context) []uuid.UUID {
	ids, err := pq.IDs(ctx)
	if err != nil {
		panic(err)
	}
	return ids
}

// Count returns the count of the given query.
func (pq *PodQuery) Count(ctx context.Context) (int, error) {
	ctx = setContextOp(ctx, pq.ctx, ent.OpQueryCount)
	if err := pq.prepareQuery(ctx); err != nil {
		return 0, err
	}
	return withInterceptors[int](ctx, pq, querierCount[*PodQuery](), pq.inters)
}

// CountX is like Count, but panics if an error occurs.
func (pq *PodQuery) CountX(ctx context.Context) int {
	count, err := pq.Count(ctx)
	if err != nil {
		panic(err)
	}
	return count
}

// Exist returns true if the query has elements in the graph.
func (pq *PodQuery) Exist(ctx context.Context) (bool, error) {
	ctx = setContextOp(ctx, pq.ctx, ent.OpQueryExist)
	switch _, err := pq.FirstID(ctx); {
	case IsNotFound(err):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("ent: check existence: %w", err)
	default:
		return true, nil
	}
}

// ExistX is like Exist, but panics if an error occurs.
func (pq *PodQuery) ExistX(ctx context.Context) bool {
	exist, err := pq.Exist(ctx)
	if err != nil {
		panic(err)
	}
	return exist
}

// Clone returns a duplicate of the PodQuery builder, including all associated steps. It can be
// used to prepare common query builders and use them differently after the clone is made.
func (pq *PodQuery) Clone() *PodQuery {
	if pq == nil {
		return nil
	}
	return &PodQuery{
		config:       pq.config,
		ctx:          pq.ctx.Clone(),
		order:        append([]pod.OrderOption{}, pq.order...),
		inters:       append([]Interceptor{}, pq.inters...),
		predicates:   append([]predicate.Pod{}, pq.predicates...),
		withResource: pq.withResource.Clone(),
		// clone intermediate query.
		sql:       pq.sql.Clone(),
		path:      pq.path,
		modifiers: append([]func(*sql.Selector){}, pq.modifiers...),
	}
}

// WithResource tells the query-builder to eager-load the nodes that are connected to
// the "resource" edge. The optional arguments are used to configure the query builder of the edge.
func (pq *PodQuery) WithResource(opts ...func(*ResourceQuery)) *PodQuery {
	query := (&ResourceClient{config: pq.config}).Query()
	for _, opt := range opts {
		opt(query)
	}
	pq.withResource = query
	return pq
}

// GroupBy is used to group vertices by one or more fields/columns.
// It is often used with aggregate functions, like: count, max, mean, min, sum.
//
// Example:
//
//	var v []struct {
//		CreatedAt time.Time `json:"created_at,omitempty"`
//		Count int `json:"count,omitempty"`
//	}
//
//	client.Pod.Query().
//		GroupBy(pod.FieldCreatedAt).
//		Aggregate(ent.Count()).
//		Scan(ctx, &v)
func (pq *PodQuery) GroupBy(field string, fields ...string) *PodGroupBy {
	pq.ctx.Fields = append([]string{field}, fields...)
	grbuild := &PodGroupBy{build: pq}
	grbuild.flds = &pq.ctx.Fields
	grbuild.label = pod.Label
	grbuild.scan = grbuild.Scan
	return grbuild
}

// Select allows the selection one or more fields/columns for the given query,
// instead of selecting all fields in the entity.
//
// Example:
//
//	var v []struct {
//		CreatedAt time.Time `json:"created_at,omitempty"`
//	}
//
//	client.Pod.Query().
//		Select(pod.FieldCreatedAt).
//		Scan(ctx, &v)
func (pq *PodQuery) Select(fields ...string) *PodSelect {
	pq.ctx.Fields = append(pq.ctx.Fields, fields...)
	sbuild := &PodSelect{PodQuery: pq}
	sbuild.label = pod.Label
	sbuild.flds, sbuild.scan = &pq.ctx.Fields, sbuild.Scan
	return sbuild
}

// Aggregate returns a PodSelect configured with the given aggregations.
func (pq *PodQuery) Aggregate(fns ...AggregateFunc) *PodSelect {
	return pq.Select().Aggregate(fns...)
}

func (pq *PodQuery) prepareQuery(ctx context.Context) error {
	for _, inter := range pq.inters {
		if inter == nil {
			return fmt.Errorf("ent: uninitialized interceptor (forgotten import ent/runtime?)")
		}
		if trv, ok := inter.(Traverser); ok {
			if err := trv.Traverse(ctx, pq); err != nil {
				return err
			}
		}
	}
	for _, f := range pq.ctx.Fields {
		if !pod.ValidColumn(f) {
			return &ValidationError{Name: f, err: fmt.Errorf("ent: invalid field %q for query", f)}
		}
	}
	if pq.path != nil {
		prev, err := pq.path(ctx)
		if err != nil {
			return err
		}
		pq.sql = prev
	}
	return nil
}

func (pq *PodQuery) sqlAll(ctx context.Context, hooks ...queryHook) ([]*Pod, error) {
	var (
		nodes       = []*Pod{}
		_spec       = pq.querySpec()
		loadedTypes = [1]bool{
			pq.withResource != nil,
		}
	)
	_spec.ScanValues = func(columns []string) ([]any, error) {
		return (*Pod).scanValues(nil, columns)
	}
	_spec.Assign = func(columns []string, values []any) error {
		node := &Pod{config: pq.config}
		nodes = append(nodes, node)
		node.Edges.loadedTypes = loadedTypes
		return node.assignValues(columns, values)
	}
	if len(pq.modifiers) > 0 {
		_spec.Modifiers = pq.modifiers
	}
	for i := range hooks {
		hooks[i](ctx, _spec)
	}
	if err := sqlgraph.QueryNodes(ctx, pq.driver, _spec); err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nodes, nil
	}
	if query := pq.withResource; query != nil {
		if err := pq.loadResource(ctx, query, nodes,
			func(n *Pod) { n.Edges.Resource = []*Resource{} },
			func(n *Pod, e *Resource) { n.Edges.Resource = append(n.Edges.Resource, e) }); err != nil {
			return nil, err
		}
	}
	return nodes, nil
}

func (pq *PodQuery) loadResource(ctx context.Context, query *ResourceQuery, nodes []*Pod, init func(*Pod), assign func(*Pod, *Resource)) error {
	edgeIDs := make([]driver.Value, len(nodes))
	byID := make(map[uuid.UUID]*Pod)
	nids := make(map[uuid.UUID]map[*Pod]struct{})
	for i, node := range nodes {
		edgeIDs[i] = node.ID
		byID[node.ID] = node
		if init != nil {
			init(node)
		}
	}
	query.Where(func(s *sql.Selector) {
		joinT := sql.Table(pod.ResourceTable)
		s.Join(joinT).On(s.C(resource.FieldID), joinT.C(pod.ResourcePrimaryKey[1]))
		s.Where(sql.InValues(joinT.C(pod.ResourcePrimaryKey[0]), edgeIDs...))
		columns := s.SelectedColumns()
		s.Select(joinT.C(pod.ResourcePrimaryKey[0]))
		s.AppendSelect(columns...)
		s.SetDistinct(false)
	})
	if err := query.prepareQuery(ctx); err != nil {
		return err
	}
	qr := QuerierFunc(func(ctx context.Context, q Query) (Value, error) {
		return query.sqlAll(ctx, func(_ context.Context, spec *sqlgraph.QuerySpec) {
			assign := spec.Assign
			values := spec.ScanValues
			spec.ScanValues = func(columns []string) ([]any, error) {
				values, err := values(columns[1:])
				if err != nil {
					return nil, err
				}
				return append([]any{new(uuid.UUID)}, values...), nil
			}
			spec.Assign = func(columns []string, values []any) error {
				outValue := *values[0].(*uuid.UUID)
				inValue := *values[1].(*uuid.UUID)
				if nids[inValue] == nil {
					nids[inValue] = map[*Pod]struct{}{byID[outValue]: {}}
					return assign(columns[1:], values[1:])
				}
				nids[inValue][byID[outValue]] = struct{}{}
				return nil
			}
		})
	})
	neighbors, err := withInterceptors[[]*Resource](ctx, query, qr, query.inters)
	if err != nil {
		return err
	}
	for _, n := range neighbors {
		nodes, ok := nids[n.ID]
		if !ok {
			return fmt.Errorf(`unexpected "resource" node returned %v`, n.ID)
		}
		for kn := range nodes {
			assign(kn, n)
		}
	}
	return nil
}

func (pq *PodQuery) sqlCount(ctx context.Context) (int, error) {
	_spec := pq.querySpec()
	if len(pq.modifiers) > 0 {
		_spec.Modifiers = pq.modifiers
	}
	_spec.Node.Columns = pq.ctx.Fields
	if len(pq.ctx.Fields) > 0 {
		_spec.Unique = pq.ctx.Unique != nil && *pq.ctx.Unique
	}
	return sqlgraph.CountNodes(ctx, pq.driver, _spec)
}

func (pq *PodQuery) querySpec() *sqlgraph.QuerySpec {
	_spec := sqlgraph.NewQuerySpec(pod.Table, pod.Columns, sqlgraph.NewFieldSpec(pod.FieldID, field.TypeUUID))
	_spec.From = pq.sql
	if unique := pq.ctx.Unique; unique != nil {
		_spec.Unique = *unique
	} else if pq.path != nil {
		_spec.Unique = true
	}
	if fields := pq.ctx.Fields; len(fields) > 0 {
		_spec.Node.Columns = make([]string, 0, len(fields))
		_spec.Node.Columns = append(_spec.Node.Columns, pod.FieldID)
		for i := range fields {
			if fields[i] != pod.FieldID {
				_spec.Node.Columns = append(_spec.Node.Columns, fields[i])
			}
		}
	}
	if ps := pq.predicates; len(ps) > 0 {
		_spec.Predicate = func(selector *sql.Selector) {
			for i := range ps {
				ps[i](selector)
			}
		}
	}
	if limit := pq.ctx.Limit; limit != nil {
		_spec.Limit = *limit
	}
	if offset := pq.ctx.Offset; offset != nil {
		_spec.Offset = *offset
	}
	if ps := pq.order; len(ps) > 0 {
		_spec.Order = func(selector *sql.Selector) {
			for i := range ps {
				ps[i](selector)
			}
		}
	}
	return _spec
}

func (pq *PodQuery) sqlQuery(ctx context.Context) *sql.Selector {
	builder := sql.Dialect(pq.driver.Dialect())
	t1 := builder.Table(pod.Table)
	columns := pq.ctx.Fields
	if len(columns) == 0 {
		columns = pod.Columns
	}
	selector := builder.Select(t1.Columns(columns...)...).From(t1)
	if pq.sql != nil {
		selector = pq.sql
		selector.Select(selector.Columns(columns...)...)
	}
	if pq.ctx.Unique != nil && *pq.ctx.Unique {
		selector.Distinct()
	}
	for _, m := range pq.modifiers {
		m(selector)
	}
	for _, p := range pq.predicates {
		p(selector)
	}
	for _, p := range pq.order {
		p(selector)
	}
	if offset := pq.ctx.Offset; offset != nil {
		// limit is mandatory for offset clause. We start
		// with default value, and override it below if needed.
		selector.Offset(*offset).Limit(math.MaxInt32)
	}
	if limit := pq.ctx.Limit; limit != nil {
		selector.Limit(*limit)
	}
	return selector
}

// ForUpdate locks the selected rows against concurrent updates, and prevent them from being
// updated, deleted or "selected ... for update" by other sessions, until the transaction is
// either committed or rolled-back.
func (pq *PodQuery) ForUpdate(opts ...sql.LockOption) *PodQuery {
	if pq.driver.Dialect() == dialect.Postgres {
		pq.Unique(false)
	}
	pq.modifiers = append(pq.modifiers, func(s *sql.Selector) {
		s.ForUpdate(opts...)
	})
	return pq
}

// ForShare behaves similarly to ForUpdate, except that it acquires a shared mode lock
// on any rows that are read. Other sessions can read the rows, but cannot modify them
// until your transaction commits.
func (pq *PodQuery) ForShare(opts ...sql.LockOption) *PodQuery {
	if pq.driver.Dialect() == dialect.Postgres {
		pq.Unique(false)
	}
	pq.modifiers = append(pq.modifiers, func(s *sql.Selector) {
		s.ForShare(opts...)
	})
	return pq
}

// Modify adds a query modifier for attaching custom logic to queries.
func (pq *PodQuery) Modify(modifiers ...func(s *sql.Selector)) *PodSelect {
	pq.modifiers = append(pq.modifiers, modifiers...)
	return pq.Select()
}

// PodGroupBy is the group-by builder for Pod entities.
type PodGroupBy struct {
	selector
	build *PodQuery
}

// Aggregate adds the given aggregation functions to the group-by query.
func (pgb *PodGroupBy) Aggregate(fns ...AggregateFunc) *PodGroupBy {
	pgb.fns = append(pgb.fns, fns...)
	return pgb
}

// Scan applies the selector query and scans the result into the given value.
func (pgb *PodGroupBy) Scan(ctx context.Context, v any) error {
	ctx = setContextOp(ctx, pgb.build.ctx, ent.OpQueryGroupBy)
	if err := pgb.build.prepareQuery(ctx); err != nil {
		return err
	}
	return scanWithInterceptors[*PodQuery, *PodGroupBy](ctx, pgb.build, pgb, pgb.build.inters, v)
}

func (pgb *PodGroupBy) sqlScan(ctx context.Context, root *PodQuery, v any) error {
	selector := root.sqlQuery(ctx).Select()
	aggregation := make([]string, 0, len(pgb.fns))
	for _, fn := range pgb.fns {
		aggregation = append(aggregation, fn(selector))
	}
	if len(selector.SelectedColumns()) == 0 {
		columns := make([]string, 0, len(*pgb.flds)+len(pgb.fns))
		for _, f := range *pgb.flds {
			columns = append(columns, selector.C(f))
		}
		columns = append(columns, aggregation...)
		selector.Select(columns...)
	}
	selector.GroupBy(selector.Columns(*pgb.flds...)...)
	if err := selector.Err(); err != nil {
		return err
	}
	rows := &sql.Rows{}
	query, args := selector.Query()
	if err := pgb.build.driver.Query(ctx, query, args, rows); err != nil {
		return err
	}
	defer rows.Close()
	return sql.ScanSlice(rows, v)
}

// PodSelect is the builder for selecting fields of Pod entities.
type PodSelect struct {
	*PodQuery
	selector
}

// Aggregate adds the given aggregation functions to the selector query.
func (ps *PodSelect) Aggregate(fns ...AggregateFunc) *PodSelect {
	ps.fns = append(ps.fns, fns...)
	return ps
}

// Scan applies the selector query and scans the result into the given value.
func (ps *PodSelect) Scan(ctx context.Context, v any) error {
	ctx = setContextOp(ctx, ps.ctx, ent.OpQuerySelect)
	if err := ps.prepareQuery(ctx); err != nil {
		return err
	}
	return scanWithInterceptors[*PodQuery, *PodSelect](ctx, ps.PodQuery, ps, ps.inters, v)
}

func (ps *PodSelect) sqlScan(ctx context.Context, root *PodQuery, v any) error {
	selector := root.sqlQuery(ctx)
	aggregation := make([]string, 0, len(ps.fns))
	for _, fn := range ps.fns {
		aggregation = append(aggregation, fn(selector))
	}
	switch n := len(*ps.selector.flds); {
	case n == 0 && len(aggregation) > 0:
		selector.Select(aggregation...)
	case n != 0 && len(aggregation) > 0:
		selector.AppendSelect(aggregation...)
	}
	rows := &sql.Rows{}
	query, args := selector.Query()
	if err := ps.driver.Query(ctx, query, args, rows); err != nil {
		return err
	}
	defer rows.Close()
	return sql.ScanSlice(rows, v)
}

// Modify adds a query modifier for attaching custom logic to queries.
func (ps *PodSelect) Modify(modifiers ...func(s *sql.Selector)) *PodSelect {
	ps.modifiers = append(ps.modifiers, modifiers...)
	return ps
}
