package tf

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Data comment
type Data struct {
	Resources []Resource `json:"resources"`
	Version   string     `json:"terraform_version"`
}

// Resource comment
type Resource struct {
	Name      string     `json:"name"`
	Instances []Instance `json:"instances"`
}

//Instance comment
type Instance struct {
	Attributes Attr `json:"attributes"`
}

// Attr comment
type Attr struct {
	Type       string      `json:"type,omitempty"`
	Domain     string      `json:"domain,omitempty"`
	Tags       TagsWrapper `json:"tags,omitempty"`
	Vars       Vars        `json:"vars,omitempty"`
	Answers    []Ans       `json:"answers,omitempty"`
	CIDRBlocks []string    `json:"cidr_blocks,omitempty"`
}

// Vars comment
type Vars struct {
	DomainName string `json:"domain_name,omitempty"`
}

// Tags comment
type Tags struct {
	Role       string `json:"Role,omitempty"`
	SearchHead string `json:"SearchHead,omitempty"`
	Stack      string `json:"Stack,omitempty"`
}

//TagsWrapper comment
type TagsWrapper struct {
	Tags
	NotArray bool `json:"-"`
}

//Ans comment
type Ans struct {
	Answer string `json:"answer"`
}

// UnmarshalJSON comment
func (t *TagsWrapper) UnmarshalJSON(data []byte) error {
	// If its an array we dont care - if you decide you want some of the tags in arrays we can expand this
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		panic("Wasnt expecting not to be able to parse this")
	}

	//fmt.Println(v)
	switch v.(type) {
	case []interface{}:
		// it's an array
		return nil
	case map[string]interface{}:
		// object so lets unmarshall it
		return json.Unmarshal(data, &t.Tags)
	case nil:
		// empty
		return nil
	default:
		panic("Wasn't expecting this")
	}
}

// ParseJSON comment
func ParseJSON(data string) map[string]string {

	cnameData := make(map[string]string)
	searchHeadMap := make(map[string]string)
	var stackName string
	testNames := make(map[string]string)
	companyDomainMap := make(map[string]string)
	var companyDomain string

	var results Data
	if err := json.Unmarshal([]byte(data), &results); err != nil {
		panic(err)
	}

	if len(results.Resources) == 0 {
		fmt.Printf("No resources found in TFstate\n")
		testNames["DELETE"] = "YES"
		return testNames
	}

	// Let's gather the company domain first, we may need it later
	for _, resource := range results.Resources {
		for _, instance := range resource.Instances {
			if instance.Attributes.Vars.DomainName != "" {
				companyDomainMap[instance.Attributes.Vars.DomainName] = "yes"
			}
		}
	}

	// Check length of map, should only be 1
	if len(companyDomainMap) > 1 {
		fmt.Printf("We've found more than one domain - %v\n", len(companyDomainMap))
	} else {
		for key := range companyDomainMap {
			companyDomain = key
		}
	}

	fmt.Printf("company Domain set - %v\n", companyDomain)

	// Let's first check that there are no whitelist rules in place, if there are, we may as well bail out here

	for _, resource := range results.Resources {
		if resource.Name == "public_search_head_sg_rules_80" || resource.Name == "public_search_head_sg_rules_443" {
			if len(resource.Instances[0].Attributes.CIDRBlocks) == 1 && resource.Instances[0].Attributes.CIDRBlocks[0] == "0.0.0.0/0" {
				fmt.Printf("No SH whitelisting found...\n")
				break
			} else {
				fmt.Printf("Whitelisting rules found...\n")
				for _, ip := range resource.Instances[0].Attributes.CIDRBlocks {
					fmt.Printf("Rule: %v\n", ip)
				}
				testNames["WHITELISTING"] = "FOUND"
				return testNames
			}
		}
	}

	for _, resource := range results.Resources {
		for _, instance := range resource.Instances {
			if instance.Attributes.Type == "CNAME" && instance.Attributes.Domain != "" {
				fmt.Println("DNS Domain found: " + instance.Attributes.Domain)
				for _, answer := range instance.Attributes.Answers {
					fmt.Printf("Alias found: %v\n", answer.Answer)
					cnameData[instance.Attributes.Domain] = answer.Answer
				}
			}

			if instance.Attributes.Tags.Role == "search-head" {
				//searchHeads = append(searchHeads, instance.Attributes.Tags.SearchHead)
				searchHeadMap[instance.Attributes.Tags.SearchHead] = "no"
				//fmt.Printf("stack name is %v\n", instance.Attributes.Tags.Stack)
				stackName = instance.Attributes.Tags.Stack
			}
		}
	}

	if len(searchHeadMap) == 0 {
		//fmt.Printf("No search heads found - nothing to do...")
		testNames["SEARCH_HEADS"] = "NONE_FOUND"
		return testNames
	}

	// Now we have our domains and aliases, let's determine what URLs we need

	fmt.Printf("Full listing of found CNAME data...\n")
	for domain, alias := range cnameData {
		fmt.Printf("Domain: %v - Alias: %v\n", domain, alias)
	}

	fmt.Printf("Looping over search-heads found in terraform...\n")

SEARCHMAP:
	for sh := range searchHeadMap {
		fmt.Printf("Checking SH: %v\n", sh)

		for domain, alias := range cnameData {
			fmt.Printf("Checking SH against - Domain: %v - Alias: %v\n", domain, alias)
			if strings.HasPrefix(cnameData[domain], sh) {
				fmt.Printf("Found a Domain for SH %v - %v\n", sh, domain)

				// We now need to extract the bits we need to later build the 1ke test
				regexString := "(.+)-" + stackName + `\.(?:stg|companyworks|companycloud)?\.(?:companycloud|com|lol)?`
				r := regexp.MustCompile(regexString)
				output := r.FindAllStringSubmatch(domain, -1)
				if len(output) > 0 {
					for _, match := range output {
						fmt.Printf("1ke ID - %v\n", match[1])
						fmt.Printf("1KE Test URL - %v\n", domain)
						key := stackName + "~" + domain
						testNames[key] = match[1]
						continue SEARCHMAP
					}

				} else {

					if sh == "sh1" {
						// it's SH1, we want the domain
						key := stackName + "~" + domain
						testNames[key] = "standard"
						continue SEARCHMAP
					}

				}

			}

		}

		fmt.Printf("Hmm, we're here for SH %v - No domain found, setting test name without domain\n", sh)

		// Not sure, let's add the test for now using our companyDomain
		// We need to construct the url and tag it standard
		var url string
		url = sh + "." + stackName + "." + companyDomain
		fmt.Printf("Test Name: %v\n", url)
		key := stackName + "~" + url
		testNames[key] = "standard"

	}

	return testNames

}
