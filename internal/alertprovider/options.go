// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package alertprovider

// Option is an alert type that the alerts of a managed endpoint can use, with what an alert inherits from its provider.
// It is an item of the alertProviders of the metadata of the endpoint form.
type Option struct {
	// Type is the alert type, the value of alerts[].type
	Type string `json:"type"`

	// Name is the readable name of the provider
	Name string `json:"name"`

	// Source is "yaml" for a provider of the configuration file and "web" for one managed through the administration
	Source string `json:"source"`

	// FailureThreshold is the failure-threshold an alert without one gets: the one of the default alert of the provider,
	// or 3
	FailureThreshold int `json:"failureThreshold"`

	// SuccessThreshold is the success-threshold an alert without one gets: the one of the default alert of the provider,
	// or 2
	SuccessThreshold int `json:"successThreshold"`

	// SendOnResolved is the send-on-resolved an alert without one gets: the one of the default alert of the provider, or
	// false
	SendOnResolved bool `json:"sendOnResolved"`

	// Description is the description an alert without one gets; empty when the default alert has none
	Description string `json:"description,omitempty"`
}

// Options returns the alert types with a valid provider, from the configuration file or the administration, in
// alphabetical order of type
func Options() []*Option {
	options := []*Option{}
	snap := current.Load()
	if snap == nil {
		return options
	}
	for _, alertType := range Types() {
		alertProvider := providerOf(snap.effective, alertType)
		if alertProvider == nil {
			continue
		}
		item := snap.item(alertType)
		option := &Option{Type: item.Type, Name: item.Name, Source: item.Source, FailureThreshold: 3, SuccessThreshold: 2}
		if defaultAlert := alertProvider.GetDefaultAlert(); defaultAlert != nil {
			if defaultAlert.FailureThreshold > 0 {
				option.FailureThreshold = defaultAlert.FailureThreshold
			}
			if defaultAlert.SuccessThreshold > 0 {
				option.SuccessThreshold = defaultAlert.SuccessThreshold
			}
			option.SendOnResolved = defaultAlert.IsSendingOnResolved()
			option.Description = defaultAlert.GetDescription()
		}
		options = append(options, option)
	}
	return options
}
