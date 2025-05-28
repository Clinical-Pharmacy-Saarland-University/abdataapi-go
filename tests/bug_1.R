library(medinfoR)
readRenviron("tests/.dev_env")

creds <- api_login(
  Sys.getenv("HOST"),
  Sys.getenv("USER"),
  Sys.getenv("PASSWORD")
)
# ddi_names <- api_interaction_compound(creds, c("Ramipril", "Hydrochlorothiazid", "Allopurinol"))

ddi_pzn <- api_interaction_pzn(creds, c("00766819", "06453257", "03399847"))
# ddi_pzn_comp <- api_interaction_pzn(creds, c("02355196", "03399847"))
