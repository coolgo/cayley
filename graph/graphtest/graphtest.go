package graphtest

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/cayleygraph/cayley/graph"
	"github.com/cayleygraph/cayley/graph/graphtest/testutil"
	"github.com/cayleygraph/cayley/graph/iterator"
	"github.com/cayleygraph/cayley/graph/path/pathtest"
	"github.com/cayleygraph/cayley/graph/shape"
	"github.com/cayleygraph/cayley/quad"
	"github.com/cayleygraph/cayley/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type Config struct {
	NoPrimitives bool
	UnTyped      bool // converts all values to Raw representation
	TimeInMs     bool
	TimeInMcs    bool
	TimeRound    bool

	OptimizesComparison bool

	AlwaysRunIntegration bool // always run integration tests

	SkipDeletedFromIterator  bool
	SkipSizeCheckAfterDelete bool
	// TODO(dennwc): these stores are not garbage-collecting nodes after quad removal
	SkipNodeDelAfterQuadDel bool
}

var graphTests = []struct {
	name string
	test func(t testing.TB, gen testutil.DatabaseFunc, conf *Config)
}{
	{"load one quad", TestLoadOneQuad},
	{"delete quad", TestDeleteQuad},
	{"sizes", TestSizes},
	{"iterator", TestIterator},
	{"hasa", TestHasA},
	{"set iterator", TestSetIterator},
	{"deleted from iterator", TestDeletedFromIterator},
	{"load typed quad", TestLoadTypedQuads},
	{"add and remove", TestAddRemove},
	{"node delete", TestNodeDelete},
	{"iterators and next result order", TestIteratorsAndNextResultOrderA},
	{"compare typed values", TestCompareTypedValues},
	{"schema", TestSchema},
}

func TestAll(t *testing.T, gen testutil.DatabaseFunc, conf *Config) {
	if conf == nil {
		conf = &Config{}
	}
	for _, gt := range graphTests {
		t.Run(gt.name, func(t *testing.T) {
			gt.test(t, gen, conf)
		})
	}
	t.Run("paths", func(t *testing.T) {
		pathtest.RunTestMorphisms(t, gen)
	})
	t.Run("integration", func(t *testing.T) {
		TestIntegration(t, gen, conf.AlwaysRunIntegration)
	})
}

// This is a simple test graph.
//
//    +---+                        +---+
//    | A |-------               ->| F |<--
//    +---+       \------>+---+-/  +---+   \--+---+
//                 ------>|#B#|      |        | E |
//    +---+-------/      >+---+      |        +---+
//    | C |             /            v
//    +---+           -/           +---+
//      ----    +---+/             |#G#|
//          \-->|#D#|------------->+---+
//              +---+
//
func MakeQuadSet() []quad.Quad {
	return []quad.Quad{
		quad.Make("A", "follows", "B", nil),
		quad.Make("C", "follows", "B", nil),
		quad.Make("C", "follows", "D", nil),
		quad.Make("D", "follows", "B", nil),
		quad.Make("B", "follows", "F", nil),
		quad.Make("F", "follows", "G", nil),
		quad.Make("D", "follows", "G", nil),
		quad.Make("E", "follows", "F", nil),
		quad.Make("B", "status", "cool", "status_graph"),
		quad.Make("D", "status", "cool", "status_graph"),
		quad.Make("G", "status", "cool", "status_graph"),
	}
}

func IteratedQuads(t testing.TB, qs graph.QuadStore, it graph.Iterator) []quad.Quad {
	ctx := context.TODO()
	var res quad.ByQuadString
	for it.Next(ctx) {
		res = append(res, qs.Quad(it.Result()))
	}
	require.Nil(t, it.Err())
	sort.Sort(res)
	return res
}

func ExpectIteratedQuads(t testing.TB, qs graph.QuadStore, it graph.Iterator, exp []quad.Quad, sortQuads bool) {
	got := IteratedQuads(t, qs, it)
	if sortQuads {
		sort.Sort(quad.ByQuadString(exp))
		sort.Sort(quad.ByQuadString(got))
	}
	require.Equal(t, exp, got)
}

func ExpectIteratedRawStrings(t testing.TB, qs graph.QuadStore, it graph.Iterator, exp []string) {
	//sort.Strings(exp)
	got := IteratedStrings(t, qs, it)
	//sort.Strings(got)
	require.Equal(t, exp, got)
}

func ExpectIteratedValues(t testing.TB, qs graph.QuadStore, it graph.Iterator, exp []quad.Value) {
	//sort.Strings(exp)
	got := IteratedValues(t, qs, it)
	//sort.Strings(got)
	require.Equal(t, len(exp), len(got), "%v\nvs\n%v", exp, got)
	for i := range exp {
		if eq, ok := exp[i].(quad.Equaler); ok {
			require.True(t, eq.Equal(got[i]))
		} else {
			require.True(t, exp[i] == got[i])
		}
	}
}

func IteratedStrings(t testing.TB, qs graph.QuadStore, it graph.Iterator) []string {
	ctx := context.TODO()
	var res []string
	for it.Next(ctx) {
		res = append(res, quad.ToString(qs.NameOf(it.Result())))
	}
	require.Nil(t, it.Err())
	sort.Strings(res)
	return res
}

func IteratedValues(t testing.TB, qs graph.QuadStore, it graph.Iterator) []quad.Value {
	ctx := context.TODO()
	var res []quad.Value
	for it.Next(ctx) {
		res = append(res, qs.NameOf(it.Result()))
	}
	require.Nil(t, it.Err())
	sort.Sort(quad.ByValueString(res))
	return res
}

func TestLoadOneQuad(t testing.TB, gen testutil.DatabaseFunc, c *Config) {
	qs, opts, closer := gen(t)
	defer closer()

	w := testutil.MakeWriter(t, qs, opts)

	q := quad.Make(
		"Something",
		"points_to",
		"Something Else",
		"context",
	)

	err := w.AddQuad(q)
	require.NoError(t, err)
	for _, pq := range []quad.String{"Something", "points_to", "Something Else", "context"} {
		tok := qs.ValueOf(pq)
		require.NotNil(t, tok, "quad store failed to find value: %q", pq)
		val := qs.NameOf(tok)
		require.NotNil(t, val, "quad store failed to decode value: %q", pq)
		require.Equal(t, pq, val, "quad store failed to roundtrip value: %q", pq)
	}
	exp := int64(5)
	if c.NoPrimitives {
		exp = 1
	}
	require.Equal(t, exp, qs.Size(), "Unexpected quadstore size")

	ExpectIteratedQuads(t, qs, qs.QuadsAllIterator(), []quad.Quad{q}, false)
}

type ValueSizer interface {
	SizeOf(graph.Value) int64
}

func TestSizes(t testing.TB, gen testutil.DatabaseFunc, conf *Config) {
	qs, opts, closer := gen(t)
	defer closer()

	w := testutil.MakeWriter(t, qs, opts)

	err := w.AddQuadSet(MakeQuadSet())
	require.NoError(t, err)
	exp := int64(22)
	if conf.NoPrimitives {
		exp = 11
	}
	require.Equal(t, exp, qs.Size(), "Unexpected quadstore size")

	if qss, ok := qs.(ValueSizer); ok {
		s := qss.SizeOf(qs.ValueOf(quad.String("B")))
		require.Equal(t, int64(5), s, "Unexpected quadstore value size")
	}

	err = w.RemoveQuad(quad.Make(
		"A",
		"follows",
		"B",
		nil,
	))
	require.NoError(t, err)
	err = w.RemoveQuad(quad.Make(
		"A",
		"follows",
		"B",
		nil,
	))
	require.True(t, graph.IsQuadNotExist(err))
	if !conf.SkipSizeCheckAfterDelete {
		exp = int64(20)
		if conf.NoPrimitives {
			exp = 10
		}
		require.Equal(t, exp, qs.Size(), "Unexpected quadstore size after RemoveQuad")
	} else {
		require.Equal(t, int64(11), qs.Size(), "Unexpected quadstore size")
	}

	if qss, ok := qs.(ValueSizer); ok {
		s := qss.SizeOf(qs.ValueOf(quad.String("B")))
		require.Equal(t, int64(4), s, "Unexpected quadstore value size")
	}
}

func TestIterator(t testing.TB, gen testutil.DatabaseFunc, _ *Config) {
	ctx := context.TODO()
	qs, opts, closer := gen(t)
	defer closer()

	testutil.MakeWriter(t, qs, opts, MakeQuadSet()...)

	var it graph.Iterator

	it = qs.NodesAllIterator()
	require.NotNil(t, it)

	size, _ := it.Size()
	require.True(t, size > 0 && size < 23, "Unexpected size: %v", size)
	// TODO: leveldb had this test
	//if exact {
	//	t.Errorf("Got unexpected exact result.")
	//}
	require.Equal(t, graph.All, it.Type(), "Unexpected iterator type")

	optIt, changed := it.Optimize()
	require.True(t, !changed && optIt == it, "Optimize unexpectedly changed iterator: %v, %T", changed, optIt)

	expect := []string{
		"A",
		"B",
		"C",
		"D",
		"E",
		"F",
		"G",
		"follows",
		"status",
		"cool",
		"status_graph",
	}
	sort.Strings(expect)
	for i := 0; i < 2; i++ {
		got := IteratedStrings(t, qs, it)
		sort.Strings(got)
		require.Equal(t, expect, got, "Unexpected iterated result on repeat %d", i)
		it.Reset()
	}

	for _, pq := range expect {
		ok := it.Contains(ctx, qs.ValueOf(quad.Raw(pq)))
		require.NoError(t, it.Err())
		require.True(t, ok, "Failed to find and check %q correctly", pq)

	}
	// FIXME(kortschak) Why does this fail?
	/*
		for _, pq := range []string{"baller"} {
			if it.Contains(qs.ValueOf(pq)) {
				t.Errorf("Failed to check %q correctly", pq)
			}
		}
	*/
	it.Reset()

	it = qs.QuadsAllIterator()
	optIt, changed = it.Optimize()
	require.True(t, !changed && optIt == it, "Optimize unexpectedly changed iterator: %v, %T", changed, optIt)

	require.True(t, it.Next(ctx))

	q := qs.Quad(it.Result())
	require.Nil(t, it.Err())
	require.True(t, q.IsValid(), "Invalid quad returned: %q", q)
	set := MakeQuadSet()
	var ok bool
	for _, e := range set {
		if e.String() == q.String() {
			ok = true
			break
		}
	}
	require.True(t, ok, "Failed to find %q during iteration, got:%q", q, set)
}

func TestHasA(t testing.TB, gen testutil.DatabaseFunc, conf *Config) {
	qs, opts, closer := gen(t)
	defer closer()

	testutil.MakeWriter(t, qs, opts, MakeQuadSet()...)

	var it graph.Iterator = iterator.NewHasA(qs,
		iterator.NewLinksTo(qs, qs.NodesAllIterator(), quad.Predicate),
		quad.Predicate)
	defer it.Close()

	it, _ = it.Optimize()
	it, _ = qs.OptimizeIterator(it)

	var exp []quad.Value
	for i := 0; i < 8; i++ {
		exp = append(exp, quad.Raw("follows"))
	}
	for i := 0; i < 3; i++ {
		exp = append(exp, quad.Raw("status"))
	}
	ExpectIteratedValues(t, qs, it, exp)
}

func TestSetIterator(t testing.TB, gen testutil.DatabaseFunc, _ *Config) {
	qs, opts, closer := gen(t)
	defer closer()

	testutil.MakeWriter(t, qs, opts, MakeQuadSet()...)

	expectIteratedQuads := func(it graph.Iterator, exp []quad.Quad) {
		ExpectIteratedQuads(t, qs, it, exp, false)
	}

	// Subject iterator.
	it := qs.QuadIterator(quad.Subject, qs.ValueOf(quad.String("C")))

	expectIteratedQuads(it, []quad.Quad{
		quad.Make("C", "follows", "B", nil),
		quad.Make("C", "follows", "D", nil),
	})
	it.Reset()

	and := iterator.NewAnd(qs,
		qs.QuadsAllIterator(),
		it,
	)

	expectIteratedQuads(and, []quad.Quad{
		quad.Make("C", "follows", "B", nil),
		quad.Make("C", "follows", "D", nil),
	})

	// Object iterator.
	it = qs.QuadIterator(quad.Object, qs.ValueOf(quad.String("F")))

	expectIteratedQuads(it, []quad.Quad{
		quad.Make("B", "follows", "F", nil),
		quad.Make("E", "follows", "F", nil),
	})

	and = iterator.NewAnd(qs,
		qs.QuadIterator(quad.Subject, qs.ValueOf(quad.String("B"))),
		it,
	)

	expectIteratedQuads(and, []quad.Quad{
		quad.Make("B", "follows", "F", nil),
	})

	// Predicate iterator.
	it = qs.QuadIterator(quad.Predicate, qs.ValueOf(quad.String("status")))

	expectIteratedQuads(it, []quad.Quad{
		quad.Make("B", "status", "cool", "status_graph"),
		quad.Make("D", "status", "cool", "status_graph"),
		quad.Make("G", "status", "cool", "status_graph"),
	})

	// Label iterator.
	it = qs.QuadIterator(quad.Label, qs.ValueOf(quad.String("status_graph")))

	expectIteratedQuads(it, []quad.Quad{
		quad.Make("B", "status", "cool", "status_graph"),
		quad.Make("D", "status", "cool", "status_graph"),
		quad.Make("G", "status", "cool", "status_graph"),
	})
	it.Reset()

	// Order is important
	and = iterator.NewAnd(qs,
		qs.QuadIterator(quad.Subject, qs.ValueOf(quad.String("B"))),
		it,
	)

	expectIteratedQuads(and, []quad.Quad{
		quad.Make("B", "status", "cool", "status_graph"),
	})
	it.Reset()

	// Order is important
	and = iterator.NewAnd(qs,
		it,
		qs.QuadIterator(quad.Subject, qs.ValueOf(quad.String("B"))),
	)

	expectIteratedQuads(and, []quad.Quad{
		quad.Make("B", "status", "cool", "status_graph"),
	})
}

func TestDeleteQuad(t testing.TB, gen testutil.DatabaseFunc, _ *Config) {
	qs, opts, closer := gen(t)
	defer closer()

	w := testutil.MakeWriter(t, qs, opts, MakeQuadSet()...)

	vn := qs.ValueOf(quad.Raw("E"))
	require.NotNil(t, vn)

	it := qs.QuadIterator(quad.Subject, vn)
	ExpectIteratedQuads(t, qs, it, []quad.Quad{
		quad.Make("E", "follows", "F", nil),
	}, false)
	it.Close()

	err := w.RemoveQuad(quad.Make("E", "follows", "F", nil))
	require.NoError(t, err)

	it = qs.QuadIterator(quad.Subject, qs.ValueOf(quad.Raw("E")))
	ExpectIteratedQuads(t, qs, it, nil, false)
	it.Close()

	it = qs.QuadsAllIterator()
	ExpectIteratedQuads(t, qs, it, []quad.Quad{
		quad.Make("A", "follows", "B", nil),
		quad.Make("C", "follows", "B", nil),
		quad.Make("C", "follows", "D", nil),
		quad.Make("D", "follows", "B", nil),
		quad.Make("B", "follows", "F", nil),
		quad.Make("F", "follows", "G", nil),
		quad.Make("D", "follows", "G", nil),
		quad.Make("B", "status", "cool", "status_graph"),
		quad.Make("D", "status", "cool", "status_graph"),
		quad.Make("G", "status", "cool", "status_graph"),
	}, true)
	it.Close()
}

func TestDeletedFromIterator(t testing.TB, gen testutil.DatabaseFunc, conf *Config) {
	if conf.SkipDeletedFromIterator {
		t.SkipNow()
	}
	qs, opts, closer := gen(t)
	defer closer()

	w := testutil.MakeWriter(t, qs, opts, MakeQuadSet()...)

	// Subject iterator.
	it := qs.QuadIterator(quad.Subject, qs.ValueOf(quad.Raw("E")))

	ExpectIteratedQuads(t, qs, it, []quad.Quad{
		quad.Make("E", "follows", "F", nil),
	}, false)

	it.Reset()

	w.RemoveQuad(quad.Make("E", "follows", "F", nil))

	ExpectIteratedQuads(t, qs, it, nil, false)
}

func TestLoadTypedQuads(t testing.TB, gen testutil.DatabaseFunc, conf *Config) {
	qs, opts, closer := gen(t)
	defer closer()

	w := testutil.MakeWriter(t, qs, opts)

	values := []quad.Value{
		quad.BNode("A"), quad.IRI("name"), quad.String("B"), quad.IRI("graph"),
		quad.IRI("B"), quad.Raw("<type>"),
		quad.TypedString{Value: "10", Type: "int"},
		quad.LangString{Value: "value", Lang: "en"},
		quad.Int(-123456789),
		quad.Float(-12345e-6),
		quad.Bool(true),
		quad.Time(time.Now()),
	}

	err := w.AddQuadSet([]quad.Quad{
		{values[0], values[1], values[2], values[3]},
		{values[4], values[5], values[6], nil},
		{values[4], values[5], values[7], nil},
		{values[0], values[1], values[8], nil},
		{values[0], values[1], values[9], nil},
		{values[0], values[1], values[10], nil},
		{values[0], values[1], values[11], nil},
	})
	require.NoError(t, err)
	for _, pq := range values {
		got := qs.NameOf(qs.ValueOf(pq))
		if !conf.UnTyped {
			if pt, ok := pq.(quad.Time); ok {
				var trim int64
				if conf.TimeInMcs {
					trim = 1000
				} else if conf.TimeInMs {
					trim = 1000000
				}
				if trim > 0 {
					tm := time.Time(pt)
					seconds := tm.Unix()
					nanos := int64(tm.Sub(time.Unix(seconds, 0)))
					if conf.TimeRound {
						nanos = (nanos/trim + ((nanos/(trim/10))%10)/5) * trim
					} else {
						nanos = (nanos / trim) * trim
					}
					pq = quad.Time(time.Unix(seconds, nanos).UTC())
				}
			}
			if eq, ok := pq.(quad.Equaler); ok {
				assert.True(t, eq.Equal(got), "Failed to roundtrip %q (%T), got %q (%T)", pq, pq, got, got)
			} else {
				assert.Equal(t, pq, got, "Failed to roundtrip %q (%T)", pq, pq)
			}
			// check if we can get received value again (hash roundtrip)
			got2 := qs.NameOf(qs.ValueOf(got))
			assert.Equal(t, got, got2, "Failed to use returned value to get it again")
		} else {
			assert.Equal(t, quad.StringOf(pq), quad.StringOf(got), "Failed to roundtrip raw %q (%T)", pq, pq)
		}
	}
	exp := int64(19)
	if conf.NoPrimitives {
		exp = 7
	}
	require.Equal(t, exp, qs.Size(), "Unexpected quadstore size")
}

// TODO(dennwc): add tests to verify that QS behaves in a right way with IgnoreOptions,
// returns ErrQuadExists, ErrQuadNotExists is doing rollback.
func TestAddRemove(t testing.TB, gen testutil.DatabaseFunc, conf *Config) {
	qs, opts, closer := gen(t)
	defer closer()

	if opts == nil {
		opts = make(graph.Options)
	}
	opts["ignore_duplicate"] = true

	w := testutil.MakeWriter(t, qs, opts, MakeQuadSet()...)

	sz := int64(22)
	if conf.NoPrimitives {
		sz = 11
	}
	require.Equal(t, sz, qs.Size(), "Incorrect number of quads")

	all := qs.NodesAllIterator()
	expect := []string{
		"A",
		"B",
		"C",
		"D",
		"E",
		"F",
		"G",
		"cool",
		"follows",
		"status",
		"status_graph",
	}
	ExpectIteratedRawStrings(t, qs, all, expect)

	// Add more quads, some conflicts
	err := w.AddQuadSet([]quad.Quad{
		quad.Make("A", "follows", "B", nil), // duplicate
		quad.Make("F", "follows", "B", nil),
		quad.Make("C", "follows", "D", nil), // duplicate
		quad.Make("X", "follows", "B", nil),
	})
	assert.Nil(t, err, "AddQuadSet failed")

	sz = int64(25)
	if conf.NoPrimitives {
		sz = 13
	}
	assert.Equal(t, sz, qs.Size(), "Incorrect number of quads")

	all = qs.NodesAllIterator()
	expect = []string{
		"A",
		"B",
		"C",
		"D",
		"E",
		"F",
		"G",
		"X",
		"cool",
		"follows",
		"status",
		"status_graph",
	}
	ExpectIteratedRawStrings(t, qs, all, expect)

	// Remove quad
	toRemove := quad.Make("X", "follows", "B", nil)
	err = w.RemoveQuad(toRemove)
	require.Nil(t, err, "RemoveQuad failed")
	err = w.RemoveQuad(toRemove)
	require.True(t, graph.IsQuadNotExist(err), "expected not exists error, got: %v", err)

	if !conf.SkipNodeDelAfterQuadDel {
		expect = []string{
			"A",
			"B",
			"C",
			"D",
			"E",
			"F",
			"G",
			"cool",
			"follows",
			"status",
			"status_graph",
		}
	} else {
		expect = []string{
			"A",
			"B",
			"C",
			"D",
			"E",
			"F",
			"G",
			"X",
			"cool",
			"follows",
			"status",
			"status_graph",
		}
	}
	ExpectIteratedRawStrings(t, qs, all, nil)
	all = qs.NodesAllIterator()
	ExpectIteratedRawStrings(t, qs, all, expect)
}

func TestIteratorsAndNextResultOrderA(t testing.TB, gen testutil.DatabaseFunc, conf *Config) {
	ctx := context.TODO()
	qs, opts, closer := gen(t)
	defer closer()

	testutil.MakeWriter(t, qs, opts, MakeQuadSet()...)

	sz := int64(22)
	if conf.NoPrimitives {
		sz = 11
	}
	require.Equal(t, sz, qs.Size(), "Incorrect number of quads")

	fixed := iterator.NewFixed(qs.ValueOf(quad.Raw("C")))

	fixed2 := iterator.NewFixed(qs.ValueOf(quad.Raw("follows")))

	all := qs.NodesAllIterator()

	innerAnd := iterator.NewAnd(qs,
		iterator.NewLinksTo(qs, fixed2, quad.Predicate),
		iterator.NewLinksTo(qs, all, quad.Object),
	)

	hasa := iterator.NewHasA(qs, innerAnd, quad.Subject)
	outerAnd := iterator.NewAnd(qs, fixed, hasa)

	require.True(t, outerAnd.Next(ctx), "Expected one matching subtree")

	val := outerAnd.Result()
	require.Equal(t, quad.Raw("C"), qs.NameOf(val))

	var (
		got    []string
		expect = []string{"B", "D"}
	)
	for {
		got = append(got, quad.ToString(qs.NameOf(all.Result())))
		if !outerAnd.NextPath(ctx) {
			break
		}
	}
	sort.Strings(got)

	require.Equal(t, expect, got)

	require.True(t, !outerAnd.Next(ctx), "More than one possible top level output?")
}

const lt, lte, gt, gte = iterator.CompareLT, iterator.CompareLTE, iterator.CompareGT, iterator.CompareGTE

var tzero = time.Unix(time.Now().Unix(), 0)

var casesCompare = []struct {
	op     iterator.Operator
	val    quad.Value
	expect []quad.Value
}{
	{lt, quad.BNode("b"), []quad.Value{
		quad.BNode("alice"),
	}},
	{lte, quad.BNode("bob"), []quad.Value{
		quad.BNode("alice"), quad.BNode("bob"),
	}},
	{lt, quad.String("b"), []quad.Value{
		quad.String("alice"),
	}},
	{lte, quad.String("bob"), []quad.Value{
		quad.String("alice"), quad.String("bob"),
	}},
	{gte, quad.String("b"), []quad.Value{
		quad.String("bob"), quad.String("charlie"), quad.String("dani"),
	}},
	{lt, quad.IRI("b"), []quad.Value{
		quad.IRI("alice"),
	}},
	{lte, quad.IRI("bob"), []quad.Value{
		quad.IRI("alice"), quad.IRI("bob"),
	}},
	{lte, quad.IRI("bob"), []quad.Value{
		quad.IRI("alice"), quad.IRI("bob"),
	}},
	{gte, quad.Int(111), []quad.Value{
		quad.Int(112),
	}},
	{gte, quad.Int(110), []quad.Value{
		quad.Int(110), quad.Int(112),
	}},
	{lt, quad.Int(20), nil},
	{lte, quad.Int(20), []quad.Value{
		quad.Int(20),
	}},
	{lte, quad.Time(tzero.Add(time.Hour)), []quad.Value{
		quad.Time(tzero), quad.Time(tzero.Add(time.Hour)),
	}},
	{gt, quad.Time(tzero.Add(time.Hour)), []quad.Value{
		quad.Time(tzero.Add(time.Hour * 49)), quad.Time(tzero.Add(time.Hour * 24 * 365)),
	}},
}

func TestCompareTypedValues(t testing.TB, gen testutil.DatabaseFunc, conf *Config) {
	if conf.UnTyped {
		t.SkipNow()
	}
	qs, opts, closer := gen(t)
	defer closer()

	w := testutil.MakeWriter(t, qs, opts)

	t1 := tzero
	t2 := t1.Add(time.Hour)
	t3 := t2.Add(time.Hour * 48)
	t4 := t1.Add(time.Hour * 24 * 365)

	err := w.AddQuadSet([]quad.Quad{
		{quad.BNode("alice"), quad.BNode("bob"), quad.BNode("charlie"), quad.BNode("dani")},
		{quad.IRI("alice"), quad.IRI("bob"), quad.IRI("charlie"), quad.IRI("dani")},
		{quad.String("alice"), quad.String("bob"), quad.String("charlie"), quad.String("dani")},
		{quad.Int(100), quad.Int(112), quad.Int(110), quad.Int(20)},
		{quad.Time(t1), quad.Time(t2), quad.Time(t3), quad.Time(t4)},
	})
	require.NoError(t, err)

	for _, c := range casesCompare {
		it := iterator.NewComparison(qs.NodesAllIterator(), c.op, c.val, qs)
		ExpectIteratedValues(t, qs, it, c.expect)
	}

	for _, c := range casesCompare {
		s := shape.Compare(shape.AllNodes{}, c.op, c.val)
		ns, ok := shape.Optimize(s, qs)
		require.Equal(t, conf.OptimizesComparison, ok)
		if conf.OptimizesComparison {
			require.NotEqual(t, s, ns)
		} else {
			require.Equal(t, s, ns)
		}
		nit := shape.BuildIterator(qs, ns)
		ExpectIteratedValues(t, qs, nit, c.expect)
	}
}

func TestNodeDelete(t testing.TB, gen testutil.DatabaseFunc, conf *Config) {
	qs, opts, closer := gen(t)
	defer closer()

	w := testutil.MakeWriter(t, qs, opts, MakeQuadSet()...)

	del := quad.Raw("D")

	err := w.RemoveNode(del)
	require.NoError(t, err)

	exp := MakeQuadSet()
	for i := 0; i < len(exp); i++ {
		for _, d := range quad.Directions {
			if exp[i].Get(d) == del {
				exp = append(exp[:i], exp[i+1:]...)
				i--
				break
			}
		}
	}
	ExpectIteratedQuads(t, qs, qs.QuadsAllIterator(), exp, true)

	if conf.SkipNodeDelAfterQuadDel {
		return
	}
	ExpectIteratedValues(t, qs, qs.NodesAllIterator(), []quad.Value{
		quad.Raw("A"),
		quad.Raw("B"),
		quad.Raw("C"),
		quad.Raw("E"),
		quad.Raw("F"),
		quad.Raw("G"),
		quad.Raw("cool"),
		quad.Raw("follows"),
		quad.Raw("status"),
		quad.Raw("status_graph"),
	})
}

func TestSchema(t testing.TB, gen testutil.DatabaseFunc, conf *Config) {
	qs, opts, closer := gen(t)
	defer closer()

	w := testutil.MakeWriter(t, qs, opts, MakeQuadSet()...)

	type Person struct {
		_         struct{}   `quad:"@type > ex:Person"`
		ID        quad.IRI   `quad:"@id" json:"id"`
		Name      string     `quad:"ex:name" json:"name"`
		Something []quad.IRI `quad:"isParentOf < *,optional" json:"something"`
	}
	p := Person{
		ID:   quad.IRI("ex:bob"),
		Name: "Bob",
	}

	qw := graph.NewWriter(w)
	id, err := schema.WriteAsQuads(qw, p)
	require.NoError(t, err)
	err = qw.Close()
	require.NoError(t, err)
	require.Equal(t, p.ID, id)

	var p2 Person
	err = schema.LoadTo(nil, qs, &p2, id)
	require.NoError(t, err)
	require.Equal(t, p, p2)
}
