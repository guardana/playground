package report

import (
	"fmt"
	"strings"
)

// Provenance is what produced a run: the lab's own commit, the pins it ran
// under, the images on this machine tagged with those pins, and the machine. A value nobody could
// read is carried as the reason it could not be read, never left blank.
type Provenance struct {
	// Development names the development build of the enforcer the run used
	// instead of the pinned one; empty for a run at the pins.
	Development string
	Lab         string
	// Workspace is where the scenario and the files it names were read from.
	Workspace string
	Pins      []Pin
	Images    []Image
	Machine   string
}

// Pin is one variable of versions.env as the run read it.
type Pin struct {
	Name  string
	Value string
}

// Image is one image on this machine, tagged with a pin. Label is the pin its
// own label names, which the build set from a build argument and does not prove
// what went into the image; Want is the pin versions.env names.
type Image struct {
	Ref   string
	ID    string
	Label string
	Want  string
	// Missing says why the image could not be read; empty when it was.
	Missing string
	// Tree is the tree label the enforcer's build sets from the source it
	// verified; TreeWant is ENFORCER_TREE, empty for an image without one.
	Tree     string
	TreeWant string
}

// Matches reports whether the image is labelled with the pin it is tagged with.
func (i Image) Matches() bool { return i.Missing == "" && i.Label != "" && i.Label == i.Want }

// describe says what was compared: a label against a pin, never the image
// against what it was built from.
func (i Image) describe() string {
	if i.Missing != "" {
		return fmt.Sprintf("`%s`: %s", i.Ref, i.Missing)
	}
	verdict := "the label matches the pin"
	if !i.Matches() {
		verdict = fmt.Sprintf("does NOT match the pin `%s`", or(i.Want, "none"))
	}
	label := "with no label"
	if i.Label != "" {
		label = "labelled `" + i.Label + "`"
	}
	line := fmt.Sprintf("`%s` is `%s` on this machine, %s, %s", i.Ref, or(i.ID, "no id"), label, verdict)
	switch {
	case i.TreeWant == "":
		return line
	case i.Tree == i.TreeWant:
		return fmt.Sprintf("%s; tree label `%s` matches ENFORCER_TREE", line, i.Tree)
	case i.Tree == "":
		return fmt.Sprintf("%s; no tree label, does NOT match ENFORCER_TREE `%s`", line, i.TreeWant)
	}
	return fmt.Sprintf("%s; tree label `%s` does NOT match ENFORCER_TREE `%s`", line, i.Tree, i.TreeWant)
}

func writeProvenance(out *writer, p Provenance) {
	out.printf("## Provenance\n\n")
	if p.Development != "" {
		out.printf("- Enforcer: **%s**\n", p.Development)
	}
	out.printf("- Lab: %s\n", or(p.Lab, "not recorded"))
	out.printf("- Workspace: %s\n", or(p.Workspace, "not recorded"))
	if len(p.Pins) == 0 {
		out.printf("- Pins: not recorded\n")
	}
	for _, pin := range p.Pins {
		out.printf("- Pin `%s=%s`\n", pin.Name, pin.Value)
	}
	if len(p.Images) == 0 {
		out.printf("- Images: not recorded\n")
	}
	for _, image := range p.Images {
		out.printf("- Image %s\n", image.describe())
	}
	out.printf("- Machine: %s\n\n", or(p.Machine, "not recorded"))
}

func provenanceProperties(p Provenance) []junitProperty {
	var properties []junitProperty
	if p.Development != "" {
		properties = append(properties, junitProperty{Name: "enforcer.development", Value: p.Development})
	}
	properties = append(properties, []junitProperty{
		{Name: "lab", Value: or(p.Lab, "not recorded")},
		{Name: "workspace", Value: or(p.Workspace, "not recorded")},
	}...)
	for _, pin := range p.Pins {
		properties = append(properties, junitProperty{Name: "pin." + pin.Name, Value: pin.Value})
	}
	for _, image := range p.Images {
		value := image.Missing
		if value == "" {
			value = strings.TrimSpace(image.ID + " labelled=" + image.Label)
			if image.TreeWant != "" {
				value += " tree=" + image.Tree
			}
		}
		properties = append(properties, junitProperty{Name: "image." + image.Ref, Value: value})
	}
	return append(properties, junitProperty{Name: "machine", Value: or(p.Machine, "not recorded")})
}
