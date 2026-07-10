package handoff

//pantograph:f handoff-from=simple
func simpleFrom() {}

//pantograph:f handoff-to=simple
func simpleTo() {}

//pantograph:f handoff-to=a handoff-from=b cond="why"
func dualNode() {}

//pantograph:f handoff-from=a
func srcA() {}

//pantograph:f handoff-to=b
func dstB() {}
