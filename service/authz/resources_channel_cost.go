package authz

const ResourceChannelCost = "channel_cost"

var ChannelCostRead = Permission{Resource: ResourceChannelCost, Action: ActionRead}

func init() {
	RegisterResource(ResourceDefinition{
		Resource: ResourceChannelCost,
		LabelKey: "Cost & Billing",
		Actions: []ActionDefinition{{
			Action:         ActionRead,
			LabelKey:       "View cost and billing details",
			DescriptionKey: "View revenue, upstream cost and profit by channel, model, group and user.",
		}},
	})
}
