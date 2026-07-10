package typenode

//pantograph:tn kind=event handoff-from=t2f note="feed in" handoff-to=fanA
type Source struct {
	field int
}

type (
	//pantograph:tn handoff-from=s2f
	Sink struct{ a string }

	//pantograph:tn
	Relay struct{ b bool }
)

//pantograph:tn handoff-from=f2t handoff-to=s2f
func consume() {}

//pantograph:tn kind=store handoff-to=f2t handoff-to=fanB
type Done struct{}

//pantograph:tn handoff-to=t2f
func sinkFunc() {}

//pantograph:tn handoff-from=fanA note="to source" handoff-from=fanB note="to done"
func fanSource() {}
