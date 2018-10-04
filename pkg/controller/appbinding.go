package controller

func (c *Controller) initAppBindingWatcher() {
	c.appBindingInformer = c.appcatInformerFactory.Appcatalog().V1alpha1().AppBindings().Informer()
	c.appBindingLister = c.appcatInformerFactory.Appcatalog().V1alpha1().AppBindings().Lister()
}
