package authz

const (
	ResourceSessionRecorderRevenue = "session_recorder_revenue"
	ResourceSessionRecorderContent = "session_recorder_content"
)

var (
	SessionRecorderRevenueRead  = Permission{Resource: ResourceSessionRecorderRevenue, Action: ActionRead}
	SessionRecorderRevenueWrite = Permission{Resource: ResourceSessionRecorderRevenue, Action: ActionWrite}
	SessionRecorderContentRead  = Permission{Resource: ResourceSessionRecorderContent, Action: ActionRead}
)

func init() {
	RegisterResource(ResourceDefinition{
		Resource: ResourceSessionRecorderRevenue,
		LabelKey: "Session recording revenue",
		Actions: []ActionDefinition{
			{
				Action:         ActionRead,
				LabelKey:       "View session recording revenue",
				DescriptionKey: "View archived data volume and revenue of recorded sessions by rule and model.",
			},
			{
				Action:         ActionWrite,
				LabelKey:       "Edit session recording revenue rules",
				DescriptionKey: "Change the model match rules and prices used to compute session recording revenue.",
			},
		},
	})
	RegisterResource(ResourceDefinition{
		Resource: ResourceSessionRecorderContent,
		LabelKey: "Session recording content",
		Actions: []ActionDefinition{{
			Action:         ActionRead,
			LabelKey:       "View recorded conversations",
			DescriptionKey: "Browse recorded sessions and read the request and response content of each record. Every view is audited.",
		}},
	})
}
