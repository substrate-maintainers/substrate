package main

// managed by go generate; do not edit by hand

func indexTemplate() string {
	return `<!DOCTYPE html>
<html lang="en">
<meta charset="utf-8">
<title>Instance Factory</title>
<body>
<h1>Instance Factory</h1>
<p class="context">This tool provisions EC2 instances that administrators can use to work in the cloud. Alternatively, the <a href="credential-factory">Credential Factory</a> mints short-lived credentials with the same privileges as these EC2 instances that administrators can use (more) safely on their laptops.</p>
{{- if .Error}}
<p class="error">{{.Error}}</p>
{{- end}}
<form method="GET">
<p>Launch a new EC2 instance in:
{{- range $i, $region := .Regions}}
<input name="region" type="submit" value="{{$region}}">
{{- end}}
</p>
</form>
<table border="1" cellpadding="2" cellspacing="2">
<tr>
    <th>Hostname</th>
    <th>Availability Zone</th>
    <th>Instance Type</th>
    <th>Launch Time</th>
    <th>State</th>
    <th>&nbsp;</th>
</tr>
{{- $launched := .Launched}}
{{- $terminate := .Terminate}}
{{- $terminated := .Terminated}}
{{- range .Instances}}
<tr{{if eq (StringValue .InstanceId) $launched}} bgcolor="#eeffee"{{else if eq (StringValue .InstanceId) $terminate}} bgcolor="#ffeeee"{{else if eq (StringValue .InstanceId) $terminated}} bgcolor="#ffeeee"{{end}}>
    <td>{{.PublicDnsName}}</td>
    <td>{{.Placement.AvailabilityZone}}</td>
    <td>{{.InstanceType}}</td>
    <td>{{.LaunchTime}}</td>
    <td>{{.State.Name}}</td>
    <td>{{if eq (StringValue .State.Name) "running"}}{{if eq (StringValue .InstanceId) $terminate}}
        <form method="POST">
            <input type="submit" value="Yes, Terminate">
            <input name="region" type="hidden" value="{{.Placement.AvailabilityZone | RegionFromAZ}}">
            <input name="terminate" type="hidden" value="{{.InstanceId}}">
        </form>
    {{else}}<a href="?terminate={{.InstanceId}}">Terminate</a>{{end}}{{else}}&nbsp;{{end}}</td>
</tr>
{{- end}}
</table>
<hr>
<pre>{{.Debug}}</pre>
</body>
</html>
`
}
