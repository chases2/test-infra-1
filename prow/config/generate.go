// Copyright 2019 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ghodss/yaml"
	"github.com/hashicorp/go-multierror"
	"github.com/kr/pretty"
	"gopkg.in/robfig/cron.v2"
	v1 "k8s.io/api/core/v1"
	prowjob "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
)

func exit(err error, context string) {
	if context == "" {
		_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
	} else {
		_, _ = fmt.Fprintf(os.Stderr, "%v: %v\n", context, err)
	}
	os.Exit(1)
}

const (
	TestGridDashboard   = "testgrid-dashboards"
	TestGridAlertEmail  = "testgrid-alert-email"
	TestGridNumFailures = "testgrid-num-failures-to-alert"

	DefaultAutogenHeader = "# THIS FILE IS AUTOGENERATED, DO NOT EDIT IT MANUALLY."

	DefaultResource = "default"

	ModifierHidden   = "hidden"
	ModifierOptional = "optional"
	ModifierSkipped  = "skipped"

	TypePostsubmit = "postsubmit"
	TypePresubmit  = "presubmit"
	TypePeriodic   = "periodic"

	variableSubstitutionFormat = `\$\([_a-zA-Z0-9.-]+(\.[_a-zA-Z0-9.-]+)*\)`
)

var variableSubstitutionRegex = regexp.MustCompile(variableSubstitutionFormat)

type Client struct {
	GlobalConfig GlobalConfig
}

type GlobalConfig struct {
	AutogenHeader string `json:"autogen_header,omitempty"`

	PathAliases map[string]string `json:"path_aliases,omitempty"`

	Cluster      string            `json:"cluster,omitempty"`
	NodeSelector map[string]string `json:"node_selector,omitempty"`

	TestgridConfig TestgridConfig `json:"testgrid_config,omitempty"`

	Annotations map[string]string `json:"annotations,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`

	Resources          map[string]v1.ResourceRequirements `json:"resources,omitempty"`
	BaseRequirements   []string                           `json:"base_requirements,omitempty"`
	RequirementPresets map[string]RequirementPreset       `json:"requirement_presets,omitempty"`
}

type TestgridConfig struct {
	Enabled            bool   `json:"enabled,omitempty"`
	AlertEmail         string `json:"alert_email,omitempty"`
	NumFailuresToAlert string `json:"num_failures_to_alert,omitempty"`
}

type JobsConfig struct {
	Jobs []Job `json:"jobs,omitempty"`

	Repo     string   `json:"repo,omitempty"`
	Org      string   `json:"org,omitempty"`
	Branches []string `json:"branches,omitempty"`

	Matrix map[string][]string `json:"matrix,omitempty"`

	Env                     []v1.EnvVar `json:"env,omitempty"`
	Image                   string      `json:"image,omitempty"`
	ImagePullPolicy         string      `json:"image_pull_policy,omitempty"`
	SupportReleaseBranching bool        `json:"support_release_branching,omitempty"`

	Cluster      string            `json:"cluster,omitempty"`
	NodeSelector map[string]string `json:"node_selector,omitempty"`

	Annotations map[string]string `json:"annotations,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`

	Resources          map[string]v1.ResourceRequirements `json:"resources,omitempty"`
	Requirements       []string                           `json:"requirements,omitempty"`
	RequirementPresets map[string]RequirementPreset       `json:"requirement_presets,omitempty"`
}

type Job struct {
	Name    string            `json:"name,omitempty"`
	Command []string          `json:"command,omitempty"`
	Env     []v1.EnvVar       `json:"env,omitempty"`
	Types   []string          `json:"types,omitempty"`
	Timeout *prowjob.Duration `json:"timeout,omitempty"`
	Repos   []string          `json:"repos,omitempty"`

	Image                   string `json:"image,omitempty"`
	ImagePullPolicy         string `json:"image_pull_policy,omitempty"`
	DisableReleaseBranching bool   `json:"disable_release_branching,omitempty"`

	Interval       string `json:"interval,omitempty"`
	Cron           string `json:"cron,omitempty"`
	Regex          string `json:"regex,omitempty"`
	MaxConcurrency int    `json:"max_concurrency,omitempty"`

	Cluster      string            `json:"cluster,omitempty"`
	NodeSelector map[string]string `json:"node_selector,omitempty"`

	Annotations map[string]string `json:"annotations,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`

	Resources    string   `json:"resources,omitempty"`
	Modifiers    []string `json:"modifiers,omitempty"`
	Requirements []string `json:"requirements,omitempty"`
}

func ReadGlobalSettings(file string) GlobalConfig {
	yamlFile, err := ioutil.ReadFile(file)
	if err != nil {
		exit(err, "failed to read "+file)
	}
	globalSettings := GlobalConfig{
		AutogenHeader: DefaultAutogenHeader,
	}
	if err := yaml.Unmarshal(yamlFile, &globalSettings); err != nil {
		exit(err, "failed to unmarshal "+file)
	}

	return globalSettings
}

// Reads the job yaml
func (cli *Client) ReadJobConfig(file string) JobsConfig {
	yamlFile, err := ioutil.ReadFile(file)
	if err != nil {
		exit(err, "failed to read "+file)
	}
	jobs := JobsConfig{}
	if err := yaml.Unmarshal(yamlFile, &jobs); err != nil {
		exit(err, "failed to unmarshal "+file)
	}

	if len(jobs.Branches) == 0 {
		jobs.Branches = []string{"master"}
	}

	resources := cli.GlobalConfig.Resources
	if resources == nil {
		resources = map[string]v1.ResourceRequirements{}
	}
	for k, v := range jobs.Resources {
		resources[k] = v
	}
	jobs.Resources = resources

	requirementPresets := cli.GlobalConfig.RequirementPresets
	if requirementPresets == nil {
		requirementPresets = map[string]RequirementPreset{}
	}
	for k, v := range jobs.RequirementPresets {
		requirementPresets[k] = v
	}
	jobs.RequirementPresets = requirementPresets

	return jobs
}

// Writes the job yaml
func WriteJobConfig(jobsConfig JobsConfig, file string) error {
	bytes, err := yaml.Marshal(jobsConfig)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(file, bytes, 0644)
}

func (cli *Client) ValidateJobConfig(fileName string, jobsConfig JobsConfig) {
	var err error
	if jobsConfig.Image == "" {
		err = multierror.Append(err, fmt.Errorf("%s: image must be set", fileName))
	}

	requirements := make([]string, 0)
	for name := range jobsConfig.RequirementPresets {
		requirements = append(requirements, name)
	}
	for _, r := range jobsConfig.Requirements {
		if e := validate(
			r,
			requirements,
			"requirements"); e != nil {
			err = multierror.Append(err, e)
		}
	}

	for _, job := range jobsConfig.Jobs {
		if job.Resources != "" {
			if _, f := jobsConfig.Resources[job.Resources]; !f {
				err = multierror.Append(err, fmt.Errorf("%s: job '%v' has nonexistant resource '%v'", fileName, job.Name, job.Resources))
			}
		}
		for _, mod := range job.Modifiers {
			if e := validate(mod, []string{ModifierHidden, ModifierOptional, ModifierSkipped}, "status"); e != nil {
				err = multierror.Append(err, e)
			}
		}
		for _, req := range job.Requirements {
			if e := validate(
				req,
				requirements,
				"requirements"); e != nil {
				err = multierror.Append(err, e)
			}
		}
		if contains(job.Types, TypePeriodic) {
			if job.Cron != "" && job.Interval != "" {
				err = multierror.Append(err, fmt.Errorf("%s: cron and interval cannot be both set in periodic %s", fileName, job.Name))
			} else if job.Cron == "" && job.Interval == "" {
				err = multierror.Append(err, fmt.Errorf("%s: cron and interval cannot be both empty in periodic %s", fileName, job.Name))
			} else if job.Cron != "" {
				if _, e := cron.Parse(job.Cron); e != nil {
					err = multierror.Append(err, fmt.Errorf("%s: invalid cron string %s in periodic %s: %v", fileName, job.Cron, job.Name, e))
				}
			} else if job.Interval != "" {
				if _, e := time.ParseDuration(job.Interval); e != nil {
					err = multierror.Append(err, fmt.Errorf("%s: cannot parse duration %s in periodic %s: %v", fileName, job.Interval, job.Name, e))
				}
			}
		}
		for _, t := range job.Types {
			if e := validate(t, []string{TypePostsubmit, TypePresubmit, TypePeriodic}, "type"); e != nil {
				err = multierror.Append(err, e)
			}
		}
		for _, repo := range job.Repos {
			if len(strings.Split(repo, "/")) != 2 {
				err = multierror.Append(err, fmt.Errorf("%s: repo %v not valid, should take form org/repo", fileName, repo))
			}
		}
	}
	if err != nil {
		exit(err, "validation failed")
	}
}

func (cli *Client) ConvertJobConfig(jobsConfig JobsConfig, branch string) config.JobConfig {
	settings := cli.GlobalConfig
	testgridConfig := settings.TestgridConfig

	var presubmits []config.Presubmit
	var postsubmits []config.Postsubmit
	var periodics []config.Periodic

	output := config.JobConfig{
		PresubmitsStatic:  map[string][]config.Presubmit{},
		PostsubmitsStatic: map[string][]config.Postsubmit{},
		Periodics:         []config.Periodic{},
	}
	for _, parentJob := range jobsConfig.Jobs {
		expandedJobs := applyMatrixJob(parentJob, jobsConfig.Matrix)
		for _, job := range expandedJobs {
			brancher := config.Brancher{
				Branches: []string{fmt.Sprintf("^%s$", branch)},
			}

			testgridJobPrefix := jobsConfig.Org
			if branch != "master" {
				testgridJobPrefix += "_" + branch
			}
			testgridJobPrefix += "_" + jobsConfig.Repo

			requirements := settings.BaseRequirements
			for _, req := range append(job.Requirements, jobsConfig.Requirements...) {
				if !contains(requirements, req) {
					requirements = append(requirements, req)
				}
			}

			if len(job.Types) == 0 || contains(job.Types, TypePresubmit) {
				name := fmt.Sprintf("%s_%s", job.Name, jobsConfig.Repo)
				if branch != "master" {
					name += "_" + branch
				}

				presubmit := config.Presubmit{
					JobBase:   createJobBase(settings, jobsConfig, job, name, branch, jobsConfig.Resources),
					AlwaysRun: true,
					Brancher:  brancher,
				}
				if pa, ok := settings.PathAliases[jobsConfig.Org]; ok {
					presubmit.UtilityConfig.PathAlias = fmt.Sprintf("%s/%s", pa, jobsConfig.Repo)
				}
				if job.Regex != "" {
					presubmit.RegexpChangeMatcher = config.RegexpChangeMatcher{
						RunIfChanged: job.Regex,
					}
					presubmit.AlwaysRun = false
				}
				if testgridConfig.Enabled {
					presubmit.JobBase.Annotations[TestGridDashboard] = testgridJobPrefix
				}
				applyModifiersPresubmit(&presubmit, job.Modifiers)
				applyRequirements(&presubmit.JobBase, requirements, settings.RequirementPresets)
				presubmits = append(presubmits, presubmit)
			}

			if len(job.Types) == 0 || contains(job.Types, TypePostsubmit) {
				name := fmt.Sprintf("%s_%s", job.Name, jobsConfig.Repo)
				if branch != "master" {
					name += "_" + branch
				}
				name += "_postsubmit"

				postsubmit := config.Postsubmit{
					JobBase:  createJobBase(settings, jobsConfig, job, name, branch, jobsConfig.Resources),
					Brancher: brancher,
				}
				if pa, ok := settings.PathAliases[jobsConfig.Org]; ok {
					postsubmit.UtilityConfig.PathAlias = fmt.Sprintf("%s/%s", pa, jobsConfig.Repo)
				}
				if job.Regex != "" {
					postsubmit.RegexpChangeMatcher = config.RegexpChangeMatcher{
						RunIfChanged: job.Regex,
					}
				}
				if testgridConfig.Enabled {
					postsubmit.JobBase.Annotations[TestGridDashboard] = testgridJobPrefix + "_postsubmit"
					postsubmit.JobBase.Annotations[TestGridAlertEmail] = testgridConfig.AlertEmail
					postsubmit.JobBase.Annotations[TestGridNumFailures] = testgridConfig.NumFailuresToAlert
				}
				applyModifiersPostsubmit(&postsubmit, job.Modifiers)
				applyRequirements(&postsubmit.JobBase, requirements, settings.RequirementPresets)
				postsubmits = append(postsubmits, postsubmit)
			}

			if contains(job.Types, TypePeriodic) {
				name := fmt.Sprintf("%s_%s", job.Name, jobsConfig.Repo)
				if branch != "master" {
					name += "_" + branch
				}
				name += "_periodic"

				// If no repos are provided, add itself to the repo list.
				if len(job.Repos) == 0 {
					job.Repos = []string{jobsConfig.Org + "/" + jobsConfig.Repo}
				}
				periodic := config.Periodic{
					JobBase:  createJobBase(settings, jobsConfig, job, name, branch, jobsConfig.Resources),
					Interval: job.Interval,
					Cron:     job.Cron,
				}
				if testgridConfig.Enabled {
					periodic.JobBase.Annotations[TestGridDashboard] = testgridJobPrefix + "_periodic"
					periodic.JobBase.Annotations[TestGridAlertEmail] = testgridConfig.AlertEmail
					periodic.JobBase.Annotations[TestGridNumFailures] = testgridConfig.NumFailuresToAlert
				}
				applyRequirements(&periodic.JobBase, requirements, settings.RequirementPresets)
				periodics = append(periodics, periodic)
			}
		}

		if len(presubmits) > 0 {
			output.PresubmitsStatic[fmt.Sprintf("%s/%s", jobsConfig.Org, jobsConfig.Repo)] = presubmits
		}
		if len(postsubmits) > 0 {
			output.PostsubmitsStatic[fmt.Sprintf("%s/%s", jobsConfig.Org, jobsConfig.Repo)] = postsubmits
		}
		if len(periodics) > 0 {
			output.Periodics = periodics
		}
	}
	return output
}

func (cli *Client) CheckConfig(jobs config.JobConfig, currentConfigFile string) error {
	current, err := ioutil.ReadFile(currentConfigFile)
	if err != nil {
		return fmt.Errorf("failed to read current config for %s: %v", currentConfigFile, err)
	}

	newConfig, err := yaml.Marshal(jobs)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %v", err)
	}
	output := []byte(cli.GlobalConfig.AutogenHeader)
	output = append(output, newConfig...)

	if !bytes.Equal(current, output) {
		return fmt.Errorf("generated config is different than file %v", currentConfigFile)
	}
	return nil
}

func (cli *Client) WriteConfig(jobs config.JobConfig, fname string) {
	bs, err := yaml.Marshal(jobs)
	if err != nil {
		exit(err, "failed to marshal result")
	}
	dir := filepath.Dir(fname)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		exit(err, "failed to create directory: "+dir)
	}
	output := []byte(cli.GlobalConfig.AutogenHeader)
	output = append(output, bs...)
	err = ioutil.WriteFile(fname, output, 0644)
	if err != nil {
		exit(err, "failed to write result")
	}
}

func (cli *Client) PrintConfig(c interface{}) {
	bs, err := yaml.Marshal(c)
	if err != nil {
		exit(err, "failed to write result")
	}
	fmt.Println(string(bs))
}

func validate(input string, options []string, description string) error {
	valid := false
	for _, opt := range options {
		if input == opt {
			valid = true
		}
	}
	if !valid {
		return fmt.Errorf("'%v' is not a valid %v. Must be one of %v", input, description, strings.Join(options, ", "))
	}
	return nil
}

func (cli *Client) DiffConfig(result config.JobConfig, existing config.JobConfig) {
	fmt.Println("Presubmit diff:")
	diffConfigPresubmit(result, existing)
	fmt.Println("\n\nPostsubmit diff:")
	diffConfigPostsubmit(result, existing)
}

// FilterReleaseBranchingJobs filters then returns jobs with release branching enabled.
func FilterReleaseBranchingJobs(jobs []Job) []Job {
	jobsF := []Job{}
	for _, j := range jobs {
		if j.DisableReleaseBranching {
			continue
		}
		jobsF = append(jobsF, j)
	}
	return jobsF
}

func getPresubmit(c config.JobConfig, jobName string) *config.Presubmit {
	presubmits := c.PresubmitsStatic
	for _, jobs := range presubmits {
		for _, job := range jobs {
			if job.Name == jobName {
				return &job
			}
		}
	}
	return nil
}

func diffConfigPresubmit(result config.JobConfig, pj config.JobConfig) {
	known := make(map[string]struct{})
	for _, jobs := range result.PresubmitsStatic {
		for _, job := range jobs {
			known[job.Name] = struct{}{}
			current := getPresubmit(pj, job.Name)
			if current == nil {
				fmt.Println("\nCreated unknown presubmit job", job.Name)
				continue
			}
			diff := pretty.Diff(current, &job)
			if len(diff) > 0 {
				fmt.Println("\nDiff for", job.Name)
			}
			for _, d := range diff {
				fmt.Println(d)
			}
		}
	}
	for _, jobs := range pj.PresubmitsStatic {
		for _, job := range jobs {
			if _, f := known[job.Name]; !f {
				fmt.Println("Missing", job.Name)
			}
		}
	}
}

func diffConfigPostsubmit(result config.JobConfig, pj config.JobConfig) {
	known := make(map[string]struct{})
	allCurrentPostsubmits := []config.Postsubmit{}
	for _, jobs := range pj.PostsubmitsStatic {
		allCurrentPostsubmits = append(allCurrentPostsubmits, jobs...)
	}
	for _, jobs := range result.PostsubmitsStatic {
		for _, job := range jobs {
			known[job.Name] = struct{}{}
			var current *config.Postsubmit
			for _, ps := range allCurrentPostsubmits {
				if ps.Name == job.Name {
					current = &ps
					break
				}
			}
			if current == nil {
				fmt.Println("\nCreated unknown job:", job.Name)
				continue

			}
			diff := pretty.Diff(current, &job)
			if len(diff) > 0 {
				fmt.Println("\nDiff for", job.Name)
			}
			for _, d := range diff {
				fmt.Println(d)
			}
		}
	}

	for _, job := range allCurrentPostsubmits {
		if _, f := known[job.Name]; !f {
			fmt.Println("Missing", job.Name)
		}
	}
}

func createContainer(jobConfig JobsConfig, job Job, resources map[string]v1.ResourceRequirements) []v1.Container {
	img := job.Image
	if img == "" {
		img = jobConfig.Image
	}

	imgPullPolicy := job.ImagePullPolicy
	if imgPullPolicy == "" {
		imgPullPolicy = jobConfig.ImagePullPolicy
	}

	envs := job.Env
	if len(envs) == 0 {
		envs = jobConfig.Env
	} else {
		// TODO: overwrite the env with the same name
		envs = append(envs, jobConfig.Env...)
	}

	c := v1.Container{
		Image:           img,
		SecurityContext: &v1.SecurityContext{Privileged: newTrue()},
		Command:         job.Command,
		Env:             envs,
	}
	if imgPullPolicy != "" {
		c.ImagePullPolicy = v1.PullPolicy(imgPullPolicy)
	}
	jobResource := DefaultResource
	if job.Resources != "" {
		jobResource = job.Resources
	}
	if _, ok := resources[jobResource]; ok {
		c.Resources = resources[jobResource]
	}

	return []v1.Container{c}
}

func createJobBase(globalConfig GlobalConfig, jobConfig JobsConfig, job Job,
	name string, branch string, resources map[string]v1.ResourceRequirements) config.JobBase {
	yes := true
	jb := config.JobBase{
		Name:           name,
		MaxConcurrency: job.MaxConcurrency,
		Spec: &v1.PodSpec{
			Containers: createContainer(jobConfig, job, resources),
		},
		UtilityConfig: config.UtilityConfig{
			Decorate:  &yes,
			ExtraRefs: createExtraRefs(job.Repos, branch, globalConfig.PathAliases),
		},
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
	}

	if globalConfig.NodeSelector != nil {
		jb.Spec.NodeSelector = globalConfig.NodeSelector
	}
	if jobConfig.NodeSelector != nil {
		jb.Spec.NodeSelector = jobConfig.NodeSelector
	}
	if job.NodeSelector != nil {
		jb.Spec.NodeSelector = job.NodeSelector
	}

	if globalConfig.Annotations != nil {
		jb.Annotations = globalConfig.Annotations
	}
	if jobConfig.Annotations != nil {
		jb.Annotations = mergeMaps(jb.Annotations, jobConfig.Annotations)
	}
	if job.Annotations != nil {
		jb.Annotations = mergeMaps(jb.Annotations, job.Annotations)
	}

	if globalConfig.Labels != nil {
		jb.Labels = globalConfig.Labels
	}
	if jobConfig.Labels != nil {
		jb.Labels = mergeMaps(jb.Labels, jobConfig.Labels)
	}
	if job.Labels != nil {
		jb.Labels = mergeMaps(jb.Labels, job.Labels)
	}

	if job.Timeout != nil {
		jb.DecorationConfig = &prowjob.DecorationConfig{
			Timeout: job.Timeout,
		}
	}

	if globalConfig.Cluster != "" && globalConfig.Cluster != "default" {
		jb.Cluster = globalConfig.Cluster
	}
	if jobConfig.Cluster != "" && jobConfig.Cluster != "default" {
		jb.Cluster = jobConfig.Cluster
	}
	if job.Cluster != "" && job.Cluster != "default" {
		jb.Cluster = job.Cluster
	}
	return jb
}

func createExtraRefs(extraRepos []string, defaultBranch string, pathAliases map[string]string) []prowjob.Refs {
	refs := []prowjob.Refs{}
	for _, extraRepo := range extraRepos {
		branch := defaultBranch
		repobranch := strings.Split(extraRepo, "@")
		if len(repobranch) > 1 {
			branch = repobranch[1]
		}
		orgrepo := strings.Split(repobranch[0], "/")
		org, repo := orgrepo[0], orgrepo[1]
		ref := prowjob.Refs{
			Org:     org,
			Repo:    repo,
			BaseRef: branch,
		}
		if pa, ok := pathAliases[org]; ok {
			ref.PathAlias = fmt.Sprintf("%s/%s", pa, repo)
		}
		refs = append(refs, ref)
	}
	return refs
}

func applyRequirements(job *config.JobBase, requirements []string, presetMap map[string]RequirementPreset) {
	presets := make([]RequirementPreset, 0)
	for _, req := range requirements {
		presets = append(presets, presetMap[req])
	}
	resolveRequirements(job.Labels, job.Spec, presets)
}

func applyModifiersPresubmit(presubmit *config.Presubmit, jobModifiers []string) {
	for _, modifier := range jobModifiers {
		if modifier == ModifierOptional {
			presubmit.Optional = true
		} else if modifier == ModifierHidden {
			presubmit.SkipReport = true
		} else if modifier == ModifierSkipped {
			presubmit.AlwaysRun = false
		}
	}
}

func applyModifiersPostsubmit(postsubmit *config.Postsubmit, jobModifiers []string) {
	for _, modifier := range jobModifiers {
		if modifier == ModifierOptional {
			// Does not exist on postsubmit
		} else if modifier == ModifierHidden {
			postsubmit.SkipReport = true
		}
		// Cannot skip a postsubmit; instead just make `type: presubmit`
	}
}

func applyMatrixJob(job Job, matrix map[string][]string) []Job {
	yamlStr, err := yaml.Marshal(job)
	if err != nil {
		exit(err, "failed to marshal the given Job")
	}
	expandedYamlStr := applyMatrix(string(yamlStr), matrix)
	jobs := make([]Job, 0)
	for _, jobYaml := range expandedYamlStr {
		job := &Job{}
		if err := yaml.Unmarshal([]byte(jobYaml), job); err != nil {
			exit(err, "failed to unmarshal the yaml to Job")
		}
		jobs = append(jobs, *job)
	}
	return jobs
}

func applyMatrix(yamlStr string, matrix map[string][]string) []string {
	subsExps := getVarSubstitutionExpressions(yamlStr)
	if len(subsExps) == 0 {
		return []string{yamlStr}
	}

	combs := make([]string, 0)
	for _, exp := range subsExps {
		exp = strings.TrimPrefix(exp, "matrix.")
		if _, ok := matrix[exp]; ok {
			combs = append(combs, exp)
		}
	}

	res := &[]string{}
	resolveCombinations(combs, yamlStr, 0, matrix, res)
	return *res
}

func resolveCombinations(combs []string, dest string, start int, matrix map[string][]string, res *[]string) {
	if start == len(combs) {
		*res = append(*res, dest)
		return
	}

	lst := matrix[combs[start]]
	for i := range lst {
		dest := replace(dest, combs[start], lst[i])
		resolveCombinations(combs, dest, start+1, matrix, res)
	}
}

func replace(str, expKey, expVal string) string {
	return strings.ReplaceAll(str, "$(matrix."+expKey+")", expVal)
}

// getVarSubstitutionExpressions extracts all the value between "$(" and ")""
func getVarSubstitutionExpressions(yamlStr string) []string {
	allExpressions := validateString(yamlStr)
	return allExpressions
}

func validateString(value string) []string {
	expressions := variableSubstitutionRegex.FindAllString(value, -1)
	if expressions == nil {
		return nil
	}
	var result []string
	set := map[string]bool{}
	for _, expression := range expressions {
		expression = stripVarSubExpression(expression)
		if _, ok := set[expression]; !ok {
			result = append(result, expression)
			set[expression] = true
		}
	}
	return result
}

func stripVarSubExpression(expression string) string {
	return strings.TrimSuffix(strings.TrimPrefix(expression, "$("), ")")
}

// Reads the generate job config for comparison
func ReadProwJobConfig(file string) config.JobConfig {
	yamlFile, err := ioutil.ReadFile(file)
	if err != nil {
		exit(err, "failed to read "+file)
	}
	jobs := config.JobConfig{}
	if err := yaml.Unmarshal(yamlFile, &jobs); err != nil {
		exit(err, "failed to unmarshal "+file)
	}
	return jobs
}

// kubernetes API requires a pointer to a bool for some reason
func newTrue() *bool {
	b := true
	return &b
}

func contains(slice []string, item string) bool {
	set := make(map[string]struct{}, len(slice))
	for _, s := range slice {
		set[s] = struct{}{}
	}

	_, ok := set[item]
	return ok
}

// mergeMaps will merge the two maps into one.
// If there are duplicated keys in the two maps, the value in mp2 will overwrite that of mp1.
func mergeMaps(mp1, mp2 map[string]string) map[string]string {
	newMap := make(map[string]string, len(mp1))
	for k, v := range mp1 {
		newMap[k] = v
	}
	for k, v := range mp2 {
		newMap[k] = v
	}
	return newMap
}
