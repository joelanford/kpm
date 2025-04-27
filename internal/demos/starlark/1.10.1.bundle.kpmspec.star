load("@kpm/v1alpha1/registry_v1", "build", "bundle_directory")
load("@kpm/v1alpha1/cfg", "cfg")

release = "1"
dist = ".okd.4.18"
repo_namespace = "quay.io/joelanford"

build(
  release = release + dist,
  image = repo_namespace + "/kpmdemo:latest" + "-" + release,
  source = bundle_directory(
     root = "./1.10.1"
  )
)

#spec(
#  release = "1",
#  source = bundle_directory(
#    path = "./1.10.1"
#  )
#)