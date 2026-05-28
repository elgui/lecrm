package config

var Templates = map[string]MethodologyConfig{
	"gbconsult-default": GBConsultDefault,
}

var GBConsultDefault = MethodologyConfig{
	TemplateName: "gbconsult-default",
	AcquisitionChannels: []AcquisitionChannel{
		{
			Key:   "referral",
			Label: "Referral / Word-of-mouth",
			Attribution: map[string]string{
				"source": "partner",
				"medium": "referral",
			},
		},
		{
			Key:   "inbound-web",
			Label: "Inbound Web (SEO / Content)",
			Attribution: map[string]string{
				"source": "website",
				"medium": "organic",
			},
		},
		{
			Key:   "linkedin-outbound",
			Label: "LinkedIn Outbound",
			Attribution: map[string]string{
				"source": "linkedin",
				"medium": "outbound",
			},
		},
		{
			Key:   "event-networking",
			Label: "Event / Networking",
			Attribution: map[string]string{
				"source": "event",
				"medium": "in-person",
			},
		},
		{
			Key:   "paid-ads",
			Label: "Paid Advertising",
			Attribution: map[string]string{
				"source": "ads",
				"medium": "paid",
			},
		},
	},
	PipelineStages: []PipelineStage{
		{
			Key:             "discovery",
			Label:           "Discovery",
			OrderIndex:      1,
			EntryConditions: []string{},
			ExitConditions:  []string{"contact_info_complete", "budget_range_identified"},
			StageProperties: []StageProperty{
				{Key: "contact_name", Type: "string", Required: true},
				{Key: "company_name", Type: "string", Required: true},
				{Key: "acquisition_channel", Type: "enum", Required: true, AllowedValues: []string{
					"referral", "inbound-web", "linkedin-outbound", "event-networking", "paid-ads",
				}},
				{Key: "estimated_budget", Type: "number", Required: false},
				{Key: "first_contact_date", Type: "date", Required: true},
			},
		},
		{
			Key:             "qualified",
			Label:           "Qualified",
			OrderIndex:      2,
			EntryConditions: []string{"contact_info_complete", "budget_range_identified"},
			ExitConditions:  []string{"decision_maker_identified", "timeline_confirmed", "needs_documented"},
			StageProperties: []StageProperty{
				{Key: "decision_maker", Type: "string", Required: true},
				{Key: "budget_confirmed", Type: "number", Required: true},
				{Key: "expected_close_date", Type: "date", Required: true},
				{Key: "pain_points", Type: "string", Required: true},
				{Key: "competitor_involved", Type: "boolean", Required: false},
			},
		},
		{
			Key:             "proposal-sent",
			Label:           "Proposal Sent",
			OrderIndex:      3,
			EntryConditions: []string{"decision_maker_identified", "needs_documented"},
			ExitConditions:  []string{"proposal_reviewed", "feedback_received"},
			StageProperties: []StageProperty{
				{Key: "proposal_amount", Type: "number", Required: true},
				{Key: "proposal_sent_date", Type: "date", Required: true},
				{Key: "proposal_document_url", Type: "string", Required: false},
				{Key: "discount_percentage", Type: "number", Required: false},
			},
		},
		{
			Key:             "negotiation",
			Label:           "Negotiation",
			OrderIndex:      4,
			EntryConditions: []string{"proposal_reviewed"},
			ExitConditions:  []string{"terms_agreed", "contract_ready"},
			StageProperties: []StageProperty{
				{Key: "negotiation_notes", Type: "string", Required: true},
				{Key: "revised_amount", Type: "number", Required: false},
				{Key: "objections", Type: "string", Required: false},
				{Key: "next_meeting_date", Type: "date", Required: false},
			},
		},
		{
			Key:             "closed-won-lost",
			Label:           "Closed-Won/Lost",
			OrderIndex:      5,
			EntryConditions: []string{"terms_agreed"},
			ExitConditions:  []string{},
			StageProperties: []StageProperty{
				{Key: "outcome", Type: "enum", Required: true, AllowedValues: []string{"won", "lost"}},
				{Key: "final_amount", Type: "number", Required: true},
				{Key: "close_date", Type: "date", Required: true},
				{Key: "loss_reason", Type: "string", Required: false},
				{Key: "won_notes", Type: "string", Required: false},
			},
		},
	},
	Automations: []Automation{
		{
			Key: "discovery-to-qualified-notify",
			Trigger: Trigger{
				Type: "stage_transition",
				From: "discovery",
				To:   "qualified",
			},
			Actions: []Action{
				{Type: "notify", Channel: "email", Template: "deal_qualified"},
			},
		},
		{
			Key: "proposal-sent-reminder",
			Trigger: Trigger{
				Type: "stage_transition",
				From: "qualified",
				To:   "proposal-sent",
			},
			Actions: []Action{
				{Type: "notify", Channel: "email", Template: "proposal_followup_reminder"},
				{Type: "set_field", Field: "proposal_sent_date", Value: "{{now}}"},
			},
		},
		{
			Key: "closed-won-celebration",
			Trigger: Trigger{
				Type: "stage_transition",
				From: "negotiation",
				To:   "closed-won-lost",
			},
			Actions: []Action{
				{Type: "notify", Channel: "email", Template: "deal_closed_notification"},
			},
		},
	},
	ColorCoding: map[string]string{
		"discovery":       "#3B82F6",
		"qualified":       "#10B981",
		"proposal-sent":   "#F59E0B",
		"negotiation":     "#8B5CF6",
		"closed-won-lost": "#EF4444",
	},
}
