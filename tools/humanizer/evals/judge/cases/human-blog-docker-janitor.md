---
id: human-blog-docker-janitor
label: likely_human
bucket: human
source: github.com/mad01/mad01.github.io _posts/2016-08-21-lessons-learned-by-running-docker-in-production.md, commit 9c1b07de8a92 (2016-08-21)
license: owned by the repo author
generator: ""
words: 202
notes: First three paragraphs of a 2016 blog post; non-native phrasing and typos (scedule, intervalls, healty) are the human signal, the topic is one AI writes about often.
---
Short about how we run docker. The way we are running docker using aurora to scedule on mesos currently both on aws and on on premise. Every service have a aurora job defined. When a job is sceduled the service allocates resources from mesos and starts a job. That job is a set of one or multiple docker containers. This is all good and works ok.

The difficulties with running docker is not the running part. When we are working with life cycle of a service. This means that the amount of stopped containers, old version of images and unused images, and volumes, will start to eat up disk space. This is something that docker have not solved nicely yet. There is a lot of efforts from the community to come up with suggested solutions to purge junk that is building up while running it in production.

The way we have solved this is to have a small service running things on every mesos node that is running docker. This service is called `janitor` . The `janitor` is a small python service that runs jobs like docker clean commands on sceduled intervalls to keep the mesos docker nodes in a healty state.
