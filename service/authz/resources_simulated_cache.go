package authz

const ResourceSimulatedCache = "simulated_cache"

var SimulatedCacheRead = Permission{Resource: ResourceSimulatedCache, Action: ActionRead}

func init() {
	RegisterResource(ResourceDefinition{
		Resource: ResourceSimulatedCache,
		LabelKey: "Simulated cache",
		Actions: []ActionDefinition{{
			Action:         ActionRead,
			LabelKey:       "View simulated cache audit",
			DescriptionKey: "View the upstream and simulated usage of requests the simulated prompt cache rewrote, including in usage log details.",
		}},
	})
}
