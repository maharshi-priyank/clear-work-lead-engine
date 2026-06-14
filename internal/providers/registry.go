package providers

type Registry struct {
	m map[string]Adapter
}

func NewRegistry() *Registry {
	all := []Adapter{
		&GithubAdapter{},
		&RemoteOKAdapter{},
		&ProductHuntAdapter{},
		&ApolloAdapter{},
		&HunterAdapter{},
		&GoogleMapsAdapter{},
		&YelpAdapter{},
		&ProxycurlAdapter{},
		&CrunchbaseAdapter{},
	}
	m := make(map[string]Adapter, len(all))
	for _, a := range all {
		m[a.ProviderID()] = a
	}
	return &Registry{m: m}
}

func (r *Registry) Get(id string) Adapter   { return r.m[id] }
func (r *Registry) All() []Adapter          { a := make([]Adapter, 0, len(r.m)); for _, v := range r.m { a = append(a, v) }; return a }
func (r *Registry) Catalog() []CatalogEntry { a := make([]CatalogEntry, 0, len(r.m)); for _, v := range r.m { a = append(a, v.Catalog()) }; return a }
